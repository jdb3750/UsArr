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
	 *   $lib/indexerscope.svelte  the scope selection — indexers AND categories
	 *   $lib/indexercatalog       what GET /api/v1/indexers is folded into
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
		fetchIndexerCatalog,
		fetchRecentGrabs,
		grabRelease,
		openEventStream,
		problemsFrom,
		startSearch,
		type IndexerCatalog,
		type IndexerOutcome,
		type RecentGrab,
		type Release,
		type SearchReport,
		type StreamHandles
	} from '$lib/api';
	import {
		PRIORITY_NOTE,
		catalogGuidance,
		categoryLabelFor,
		categoryNames,
		categoryTree,
		clearScopeLabel,
		indexerFacts,
		indexerNames,
		indexerPickerLegend,
		indexerServices,
		indexersForInstance,
		isSearchable,
		isServiceSelectable,
		pickerScopeNote,
		resolveActiveInstance,
		scopeSummary,
		serviceFacts,
		sortIndexers,
		unavailableReason
	} from '$lib/indexercatalog';
	import { formatAge, sizeParts } from '$lib/format';
	import {
		CODE_OUTCOME_UNKNOWN,
		DEFAULT_SEARCH_TYPE,
		FROZEN_ORDER_NOTE,
		GRAB_MISSING_TITLE_NOTE,
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
		correlatedFailure,
		fanoutSummary,
		flagsAbsence,
		formatWhen,
		grabOutcome,
		grabWindow,
		isEmphasisedFlag,
		isFreeTextOnly,
		legReasons,
		liveGrabCopy,
		searchEmptyState,
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
		{ id: 'protocol', header: 'Protocol', width: '80px', stackLine: 2 },
		{ id: 'age', header: 'Age', width: '68px', align: 'end', sortable: true, stackLine: 2 },
		{
			id: 'title',
			header: 'Title',
			width: 'minmax(0, 2.4fr)',
			stackLabel: false,
			stackLine: 1
		},
		// ⚠️ THE FRACTIONS ARE MEASURED, NOT CHOSEN. At 3fr/1fr/0.8fr — the ratio
		// this table arrived with, before the Category column joined it — a 1280 px
		// viewport left Indexer 110 px, which is `Workin…` and `Newsg…`: the
		// indexer name is one of the fields a person identifies a row BY, and
		// seven characters of it identifies nothing. Screenshotted, re-weighted,
		// screenshotted again. Title keeps the largest share because it is the
		// row's identity, but it degrades gracefully — a truncated release name is
		// still recognisable from its head, and an indexer name is not.
		{ id: 'indexer', header: 'Indexer', width: 'minmax(0, 1.5fr)', stackLine: 2 },
		{ id: 'category', header: 'Category', width: 'minmax(0, 0.9fr)', stackLine: 'hidden' },
		{ id: 'size', header: 'Size', width: '88px', align: 'end', sortable: true, stackLine: 2 },
		{
			id: 'grabs',
			header: 'Grabs',
			width: '72px',
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
		{ id: 'flags', header: 'Indexer flags', width: 'minmax(0, 1.2fr)', stackLine: 'hidden' },
		// minmax(max-content, auto), never a fixed track: §9.1's overflow policy.
		// A fixed track shears the buttons attached to exactly the rows that are
		// in trouble, with no scrollbar and no way to reach what was cut.
		{ id: 'actions', header: 'Actions', width: 'minmax(max-content, auto)', stackLine: 1 }
	];

	// ⚠️ `Time`, NEVER `Downloaded`, AND THE UNION IS WHY IT MATTERS RATHER THAN
	// BEING A WORD CHOICE. The value is the time of the ATTEMPT on both arms —
	// dispatch on a provenance row, the moment the user pressed Grab on a not-sent
	// one — and UsArr stops observing at handoff, so it never learns when anything
	// finished. A column headed `Downloaded` over a not-sent row would be a claim
	// the row itself contradicts. §17.5's own word for it is "time".
	const grabColumns: ListColumn[] = [
		{ id: 'when', header: 'Time', width: '132px' },
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
	/**
	 * The type the RUNNING search was sent with, which is not the same thing as
	 * the selector's current value. The empty state's scoped copy is a claim
	 * about the search that produced these results (SW-08), so changing the
	 * selector without pressing Search must not change what the screen says
	 * about what already came back.
	 */
	let submittedType = $state(DEFAULT_SEARCH_TYPE);
	let searchId = $state<string | undefined>(undefined);
	let searching = $state(false);
	let finished = $state(false);
	let searchError = $state('');
	/** The server's own code. `no_indexer_service` is not a failure to retry —
	 * it means nothing is configured — so it gets the link that fixes it. */
	let searchErrorCode = $state('');
	/**
	 * Which indexer service the RUNNING search was resolved to, off the accepted
	 * body's own `instance_id` rather than off what the picker showed when the
	 * button was pressed. It is what the SSE frames' indexer names are filed
	 * under, and taking it from the server closes the one window where the two
	 * could disagree: a search sent with no `?instance=` at all, which the server
	 * answers by picking `candidates[0]` and telling us which that was.
	 */
	let searchedInstance = $state(0);
	let streamGap = $state('');
	let streamConnected = $state(false);
	let pickerOpen = $state(false);
	let categoryPickerOpen = $state(false);

	/* ── the scope catalogue ──────────────────────────────────────────────── */

	/**
	 * `GET /api/v1/indexers`, which is what makes the picker usable BEFORE a
	 * search has been run.
	 *
	 * ⚠️ NO LOADING STATE, NO SKELETON, AND THAT IS THE ARCHITECTURE RATHER THAN
	 * a preference. The handler reads the `indexer_catalog` replica and makes no
	 * upstream call at all (internal/httpapi/indexers.go), so this is a Tier 0
	 * local read and principle 1 says a local read renders. Drawing a spinner
	 * over it would teach the user that the picker is a thing you wait for, which
	 * is exactly the impression the replica exists to prevent.
	 *
	 * `undefined` therefore means "not answered yet" and is not rendered as a
	 * state — the picker simply draws whatever it has. What it must NOT do in
	 * that window is assert the negative: "no indexers are configured" is a claim
	 * about the install, and it is only made once `catalog` is a real answer.
	 */
	let catalog = $state<IndexerCatalog | undefined>(undefined);
	/** A genuine transport or 5xx failure, which is a different thing from any of
	 * the endpoint's four 200-borne states. */
	let catalogError = $state('');
	/** Which category parents are expanded. Ids, not indices — the tree is
	 * rebuilt whenever the indexer selection narrows it. */
	let expandedCategories = $state<number[]>([]);

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
	 * THE CORRELATED DIAGNOSIS — §17.5, "naming the non-action beats offering a
	 * fake one".
	 *
	 * Read from the CLOSING REPORT only, never from the running tally. The
	 * helper refuses a partial fan-out on its own by comparing the leg count
	 * against the server's total, and this is the second half of the same guard:
	 * before the report lands there is no authoritative total to compare against,
	 * and three legs that happened to fail first are not "every indexer".
	 *
	 * Where it fires it REPLACES the per-indexer banners rather than joining
	 * them. That is the whole point — nine banners each carrying the same title,
	 * the same two-sentence explanation and the same reason is nine renderings of
	 * one fact, and the fact worth having (that the nine are one condition) was
	 * on screen nowhere.
	 */
	const diagnosis = $derived(
		report ? correlatedFailure(report.indexers, report.totalIndexers) : undefined
	);

	/**
	 * §9.6's three states, and the middle one scoped by media type (SW-08).
	 *
	 * `answered` is what separates "your search returned nothing" from "your
	 * search returned nothing because nobody answered", and the type is what
	 * separates thin indexer coverage from a query problem. Both live in
	 * `$lib/requests` where a test can read the strings; this passes the facts in.
	 */
	const emptyCopy = $derived(
		searchEmptyState({ submitted, typeId: submittedType, answered, total: totalIndexers })
	);

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

	/* ── the scope picker's derived shape ─────────────────────────────────── */

	/**
	 * ⚠️ THE INDEXER SERVICES, AND THE ONE THIS SEARCH WILL ASK.
	 *
	 * A search asks exactly one — `?instance=` is a single id and
	 * `resolveIndexerInstance` resolves one instance per search — so the picker
	 * is scoped to one at a time and the choice is made explicitly here rather
	 * than fallen into by `candidates[0]`. `$lib/indexercatalog`'s header owns
	 * the whole argument, including the two alternatives that were refused.
	 */
	const services = $derived(indexerServices(catalog));
	/**
	 * `scope.instanceId` IS the active service, not a hint at it: `loadCatalog`
	 * resolves the stored or linked preference against the catalogue ONCE and
	 * writes the answer back, so the id the picker draws from, the id the ticks
	 * are filed under and the id `?instance=` carries are the same value and
	 * cannot drift. Resolving on every render into a separate derived would give
	 * three readers of two values, which is how a tick lands in one service and
	 * a search runs against another.
	 */
	const activeInstanceId = $derived(scope.instanceId);
	const activeService = $derived(services.find((s) => s.instanceId === activeInstanceId));
	const activeServiceName = $derived(activeService?.name ?? '');

	/**
	 * The indexers the picker offers — the ACTIVE SERVICE'S, and only those.
	 *
	 * This filter is what makes a cross-service selection unrepresentable rather
	 * than merely discouraged: there is no state of this grid that names two
	 * services, so there is no state `?instance=` cannot carry.
	 *
	 * ⚠️ THE CATALOGUE WINS OUTRIGHT WHENEVER IT HAS ROWS, and the SSE-learned
	 * union is the fallback rather than a supplement. Merging the two would keep
	 * offering an indexer that has been deleted from Prowlarr — localStorage
	 * remembers it, the catalogue does not — and a picker that offers a choice
	 * the search planner refuses is worse than one that is briefly short.
	 * `$lib/indexerscope`'s header owns the argument.
	 */
	const catalogIndexers = $derived(
		sortIndexers(indexersForInstance(catalog?.indexers ?? [], activeInstanceId))
	);
	const usingCatalogue = $derived(catalogIndexers.length > 0);

	/** Names for the scope sentence. The catalogue first, the learned union
	 * behind it, and the bare id last — a stored selection must still be
	 * describable when neither source knows the indexer any more. */
	const names = $derived(indexerNames(catalogIndexers, scope.known));

	/**
	 * The categories this search could be narrowed to, scoped to the indexers
	 * already selected — which is what makes "one tracker, one category" two
	 * decisions rather than a hunt through everything on the install.
	 */
	const tree = $derived(categoryTree(catalogIndexers, scope.selected));
	const treeNames = $derived(categoryNames(tree));

	/** THE VISIBLE STATEMENT OF SCOPE — §17.5, and the reason sticky is allowed
	 * to be sticky. The words live in `$lib/indexercatalog` where a test reads
	 * them.
	 *
	 * ⚠️ WITH TWO SERVICES CONFIGURED IT IS ALWAYS RENDERED, even on the default
	 * scope, because the default is then "every indexer in one of your two
	 * services" — a scope the user did not set, would not guess, and which the
	 * screen previously described as searching everything. */
	const scopeLine = $derived(
		scopeSummary({
			indexers: scope.selected.map((id) => ({
				id,
				name: names.get(id) ?? `indexer ${id}`
			})),
			categories: scope.categories.map((id) => ({
				id,
				// The BARE id when the tree cannot name it — a sticky selection
				// outlives the catalogue that named it, and the sentence still has to
				// describe what it is filtering so the user can clear it. Not
				// `category ${id}`: the sentence supplies the noun itself, and the
				// pair rendered "in the category 2000 category".
				name: treeNames.get(id) ?? String(id)
			})),
			knownIndexers: usingCatalogue ? catalogIndexers.length : scope.known.length,
			// Omitted until the catalogue names a service, which is the one state in
			// which the screen genuinely does not know which one it is asking.
			service:
				activeServiceName === '' ? undefined : { name: activeServiceName, total: services.length }
		})
	);

	/** The picker's own copy, which changes meaning with the number of services
	 * configured. All three live in `$lib/indexercatalog` where the banned-word
	 * guard in `requests.test.ts` reaches them. */
	const pickerLegend = $derived(indexerPickerLegend(activeServiceName, services.length));
	const scopeNote = $derived(pickerScopeNote(services.length));
	const clearLabel = $derived(clearScopeLabel(services.length));

	/** Whether the endpoint's `status` names a fix that lives on UsArr's own
	 * Services screen. Branching on `status`, never on the HTTP code: all four
	 * states arrive on a 200. */
	const catalogFixInServices = $derived(catalogGuidance(catalog?.status ?? '').fixInServices);

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
		//
		// Filed under the instance the SEARCH ran against rather than under the
		// picker's current one: the server tells us which it resolved, and an
		// indexer id learned from one Prowlarr names nothing in another.
		scope.learn(searchedInstance, outcome.indexerId, outcome.name);
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

	/**
	 * The scope picker's catalogue.
	 *
	 * Fired on mount and never awaited by anything that paints: it is a local
	 * SQLite read, so it lands in milliseconds, and the picker is drawn from
	 * whatever is in hand at the time. A failure here is recorded and the picker
	 * falls back to the SSE-learned union rather than disappearing — the scope
	 * filter is not worth taking the search screen down for.
	 */
	async function loadCatalog() {
		try {
			catalog = await fetchIndexerCatalog();
			catalogError = '';
			// ⚠️ THE ONE PLACE THE ACTIVE SERVICE IS DECIDED. The stored or linked
			// preference is held against the catalogue here — kept if that service
			// still exists and is still enabled, otherwise the first enabled one —
			// and written back, so from this point the picker, the ticks and
			// `?instance=` all read one value. Resolving it lazily at each read
			// instead would let a tick be filed under 0 while the search ran
			// against 3, which is the class of bug this whole shape exists to close.
			scope.setInstance(resolveActiveInstance(indexerServices(catalog), scope.instanceId));
		} catch (error) {
			catalogError = error instanceof ApiError ? error.detail : String(error);
		}
	}

	// One stream for the whole page, opened once. Results are appended as they
	// arrive per indexer — that progressive render is the point of the SSE design.
	onMount(() => {
		void loadGrabs();
		void loadCatalog();

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
							scope.learn(searchedInstance, outcome.indexerId, outcome.name);
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
		// ⚠️ A LINKED SCOPE IS TWO VALUES, NOT ONE. An indexer id means nothing
		// without the service it belongs to, so `?indexer=` is only adopted
		// alongside `?instance=` — a link carrying ids and no service names a
		// scope that cannot be resolved, and guessing a service for it is exactly
		// the reinterpretation `RETIRED_KEYS` refuses. The service alone is
		// adopted happily; it is the whole scope a link most often carries.
		//
		// Both run BEFORE `loadCatalog` resolves, and are then validated by it:
		// a link naming a service that has since been deleted or switched off
		// falls back to a searchable one, and the scope line says which.
		const linkedInstance = Number.parseInt(params.get('instance') ?? '', 10);
		if (Number.isSafeInteger(linkedInstance) && linkedInstance > 0) {
			scope.setInstance(linkedInstance);
			const linked = parseIds(params.getAll('indexer').join(','));
			if (linked.length > 0) scope.adopt(linkedInstance, linked);
		}

		// The same rule for the category half: a link that names categories meant
		// that scope, and it wins over whatever was left sticky.
		const linkedCategories = parseIds(params.getAll('category').join(','));
		if (linkedCategories.length > 0) scope.adoptCategories(linkedCategories);

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

	/**
	 * The query, the type, the sort and BOTH halves of the scope — all of it
	 * linkable, and all of it in the same query string ADR-0038 clause 6 already
	 * put `sort` and `dir` in. A scoped search that could not be sent to somebody
	 * is a scope that only exists in one browser.
	 */
	function syncUrl(text: string) {
		const params = new SvelteURLSearchParams();
		if (text) params.set('q', text);
		if (searchType !== DEFAULT_SEARCH_TYPE) params.set('type', searchType);
		writeSort(params, sort);
		// The service goes in FIRST and unconditionally once it is known, because
		// the indexer ids after it are only meaningful beside it. A link without
		// it is a link whose scope the receiving screen has to guess at, and it
		// deliberately will not.
		if (scope.instanceId > 0) params.set('instance', String(scope.instanceId));
		for (const id of scope.selected) params.append('indexer', String(id));
		for (const id of scope.categories) params.append('category', String(id));
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
		submittedType = searchType;
		// The service being asked, provisionally: the accepted body replaces it
		// with the one the server resolved. Reset here rather than left standing,
		// so a frame arriving between two searches is never filed under the
		// previous search's service.
		searchedInstance = scope.instanceId;
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
				indexerIds: scope.selected,
				// Narrowed server-side too: an indexer that carries none of these is
				// skipped before its leg is planned rather than queried and then
				// discarded (internal/releases/search.go, supportsAnyCategory).
				categories: scope.categories,
				// ⚠️ THE SERVICE, NAMED. Without it `resolveIndexerInstance` takes
				// `candidates[0]` by priority-then-name, which is a real choice made
				// silently — and the indexer ids above are that service's own ids, so
				// sending them to a service the client did not name is how a scoped
				// search lands on the wrong tracker. 0 means the catalogue has named
				// none, and the server's own `no_indexer_service` answer is the
				// honest one.
				instanceId: scope.instanceId
			});
			if (accepted.searchId) searchId = accepted.searchId;
			// The server's answer, not the request's hope. On a search sent with no
			// instance this is the only way to know which one ran.
			searchedInstance = accepted.instanceId ?? scope.instanceId;
			// A catalogue that could not be read leaves the picker with no service
			// at all; the search's own answer is then the first thing that names
			// one, and the SSE-learned fallback needs it to file anything.
			if (scope.instanceId <= 0 && searchedInstance > 0) scope.setInstance(searchedInstance);
		} catch (error) {
			searching = false;
			searchError = error instanceof ApiError ? error.detail : String(error);
			searchErrorCode = error instanceof ApiError ? error.code : '';
		}
	}

	/**
	 * Point the picker, the scope line and the next search at one service.
	 *
	 * It does NOT re-run the search. Changing which service is asked changes
	 * what the results would be, and silently replacing a table the user is
	 * reading — with a Grab button in every row — because they opened a picker
	 * is the reordering §9.1a forbids, applied to the whole set. The results on
	 * screen stay what they were, the scope line states what the next search
	 * will ask, and Search is the control that acts on it.
	 */
	function chooseService(id: number) {
		scope.setInstance(id);
		// The category tree is built from the active service's indexers, so a
		// parent expanded in the previous service's tree is an id that may not
		// exist in this one.
		expandedCategories = [];
		syncUrl(submitted);
	}

	function toggleIndexer(id: number) {
		scope.toggle(id);
		syncUrl(submitted);
	}

	function toggleCategory(id: number) {
		scope.toggleCategory(id);
		syncUrl(submitted);
	}

	function toggleExpanded(id: number) {
		expandedCategories = expandedCategories.includes(id)
			? expandedCategories.filter((n) => n !== id)
			: [...expandedCategories, id];
	}

	/**
	 * Back to an unscoped search, both halves at once.
	 *
	 * It clears the categories as well as the indexers, and that is the point of
	 * the control rather than a convenience: the sentence beside it states one
	 * scope, so the button under that sentence has to undo the whole of what the
	 * sentence describes. A control that left half the filter standing is how a
	 * user presses "search all indexers", sees the same thin result set, and
	 * concludes the button does nothing.
	 */
	function clearScope() {
		scope.clearAll();
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
		// The not-sent arm refreshes it as well, and that is the half this move
		// added — a grab that never left now writes an audit row the block reads.
		void loadGrabs();
	}

	/* ── Recent grabs: the one action a not-sent row may offer ────────────── */

	const requestsPath = resolve('/requests');
	const servicesPath = resolve('/services');

	/**
	 * A REAL URL, so the control is a link rather than a button wearing one.
	 *
	 * The href is what a middle-click, a ctrl-click and a right-click all act on,
	 * and it has to WORK when they do: `?q=` is the same parameter `onMount`
	 * already seeds a search from, so a new tab opened this way runs the search on
	 * its own. A button with an onclick gives a new tab nothing at all.
	 */
	function searchAgainHref(title: string): string {
		return `${requestsPath}?q=${encodeURIComponent(title)}`;
	}

	/**
	 * The same-tab half, and the reason the link keeps its href.
	 *
	 * A plain left-click is intercepted and the search runs in place: this screen
	 * IS the search, so navigating to itself would reload the SPA to do what one
	 * function call does. Every other kind of click is left to the browser —
	 * middle, modified, and any non-primary button — which is what makes the href
	 * above load-bearing rather than decoration.
	 *
	 * The query is the release title verbatim, which is the only name UsArr has
	 * for the thing that was not sent. A full release name is a narrow query, and
	 * the search box is left holding it precisely so it can be trimmed.
	 */
	function searchAgainFrom(event: MouseEvent, grab: RecentGrab) {
		if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
			return;
		}
		event.preventDefault();
		query = grab.releaseTitle;
		void runSearch();
		// Instant, never smooth: §9.1a's 0 ms rule is about not moving content
		// under an aiming pointer, and a scroll animation is exactly that. The
		// results are two blocks up, so without this the button appears to do
		// nothing at all.
		document.getElementById('release-results')?.scrollIntoView();
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
		<!--
			TWO CONTROLS, NOT ONE, AND THE WORDS ARE PROWLARR'S OWN. Prowlarr's
			search page filters by `Indexers` and by `Categories` as two separate
			pickers, and DESIGN-DIRECTION §9.1 is explicit that this vocabulary is
			taken rather than reinvented. Two short panels also keep the motivating
			case short: one tracker and one category is two openings and two ticks,
			where a single merged panel would bury the categories under the indexer
			list every time.
		-->
		<button
			type="button"
			class="btn"
			aria-expanded={pickerOpen}
			aria-controls="indexer-picker"
			onclick={() => (pickerOpen = !pickerOpen)}
		>
			{scope.isAll ? 'Indexers' : `Indexers (${scope.selected.length})`}
		</button>
		<button
			type="button"
			class="btn"
			aria-expanded={categoryPickerOpen}
			aria-controls="category-picker"
			onclick={() => (categoryPickerOpen = !categoryPickerOpen)}
		>
			{scope.isAllCategories ? 'Categories' : `Categories (${scope.categories.length})`}
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
	{#if scopeLine}
		<p class="scopeline" role="status">
			<span>{scopeLine}</span>
			<!-- The remembered-and-clearable half only where there is something to
			     remember or clear. With two services the line above is rendered on
			     the DEFAULT scope too — because "one of your two services" is not a
			     default anybody would guess — and a Clear control beside a scope
			     nobody set is a button that does nothing when pressed. -->
			{#if !scope.isAll || !scope.isAllCategories}
				<span class="muted">This selection is remembered between searches.</span>
				<button type="button" class="linkish" onclick={clearScope}>
					{clearLabel}
				</button>
			{/if}
		</p>
	{/if}

	{#if pickerOpen}
		<div class="picker" id="indexer-picker">
			<!--
				⚠️ THE SERVICE CHOICE, AND IT ONLY EXISTS WHEN THERE IS ONE TO MAKE.
				A search asks exactly one indexer service — `?instance=` is a single
				id — so this is a RADIO group rather than checkboxes: the control's
				own shape is what tells the user that picking a second is not a thing
				the request can express. On the one-service install, which is most of
				them, it is not rendered at all; naming a choice that does not exist
				is noise.

				Each option carries its own state underneath, because scoping the grid
				to one service takes away the only other place a service that UsArr
				has never read, or that answered with nothing, could have said so.
			-->
			{#if services.length > 1}
				<fieldset class="picker__set picker__set--services">
					<legend class="picker__legend">Indexer service to search</legend>
					<div class="picker__grid">
						{#each services as service (service.instanceId)}
							<label class="check pick" class:pick--off={!isServiceSelectable(service)}>
								<input
									type="radio"
									name="indexer-service"
									checked={service.instanceId === activeInstanceId}
									disabled={!isServiceSelectable(service)}
									onchange={() => chooseService(service.instanceId)}
								/>
								<span class="pick__body">
									<!-- The service's own name, verbatim: it is the user's label
									     for it and §17.5's copy rules govern UsArr's sentences,
									     never rendered data. -->
									<span class="pick__name">{service.name}</span>
									<span class="pick__sub" class:pick__sub--off={!isServiceSelectable(service)}>
										{serviceFacts(service)}
									</span>
								</span>
							</label>
						{/each}
					</div>
					<p class="picker__note muted">
						One search asks one indexer service. The indexers below are this one’s, and the ticks
						you set here are remembered per service.
					</p>
				</fieldset>
			{/if}

			<fieldset class="picker__set">
				<legend class="picker__legend">{pickerLegend}</legend>

				{#if usingCatalogue}
					<div class="picker__grid">
						<!--
							⚠️ KEYED ON THE INSTANCE AND THE INDEXER TOGETHER. The indexer id
							is the SERVICE's own id, so two configured indexer services can
							each carry an id 3 — and a key that used the indexer id alone
							would have Svelte reconcile two different rows onto one.
						-->
						{#each catalogIndexers as indexer (`${indexer.instanceId}:${indexer.indexerId}`)}
							{@const reason = unavailableReason(indexer)}
							<label class="check pick" class:pick--off={reason !== ''}>
								<!--
									⚠️ A DISABLED INDEXER IS LISTED AND MARKED, NEVER HIDDEN. The
									upstream endpoint returns enabled and disabled indexers with no
									filter parameter, and dropping the disabled ones makes "why is
									my indexer missing?" unanswerable from the one screen that
									should answer it. The control is genuinely `disabled` rather
									than merely styled: the search planner will not ask it, so a
									tick that looked as though it worked would be a filter that
									silently returns nothing. A stale selection holding one is
									recoverable from the scope line above, which names it.
								-->
								<input
									type="checkbox"
									checked={scope.isSelected(indexer.indexerId)}
									disabled={!isSearchable(indexer)}
									onchange={() => toggleIndexer(indexer.indexerId)}
								/>
								<span class="pick__body">
									<!-- The indexer's OWN name, verbatim. §17.5's copy rules govern
									     UsArr's sentences, never upstream data. -->
									<span class="pick__name">{indexer.name}</span>
									{#if indexerFacts(indexer)}
										<span class="pick__sub">{indexerFacts(indexer)}</span>
									{/if}
									{#if reason}
										<span class="pick__sub pick__sub--off">{reason}</span>
									{/if}
								</span>
							</label>
						{/each}
					</div>

					<p class="picker__note muted">
						This is your indexer service’s own list, replicated into UsArr, so it is here before you
						run a search.
						<!-- ⚠️ THE SENTENCE HERE USED TO READ "Selecting none searches all
						     of them" UNDER A GRID DRAWN FROM TWO SERVICES, while the search
						     asked one of them. `pickerScopeNote` keeps that wording for the
						     install where it is true and names the boundary where it is
						     not. -->
						{scopeNote}
						<!-- ⚠️ THE PRIORITY RULE, ONCE. The clause is identical for every
						     indexer, and §9.1 is explicit that a value identical across every
						     row of a group is stated once and dropped from the rows —
						     measured, it was three wrapped lines under every name. The
						     number itself stays on each row, where it varies. -->
						{PRIORITY_NOTE}
					</p>

					<!--
						WHERE THE LIST CAME FROM AND HOW OLD IT IS. The replica carries
						`fetched_at` per instance precisely so the screen can show staleness
						instead of paying for freshness in latency (§8.5), and §17.3 requires
						both forms of a timestamp.

						⚠️ THE ACTIVE SERVICE'S LINE, NOT EVERY SERVICE'S. It answers "how
						old is the list I am looking at?", and the list on screen came from
						one service — a read time for a service whose indexers are not
						drawn here is not staleness, it is a number beside the wrong rows.
						The others' states are on their own radio options above, which is
						where a reader is choosing between them.
					-->
					{#if activeService}
						{@const read = formatWhen(activeService.fetchedAt, now)}
						<p class="picker__note muted">
							<!-- Both forms of the timestamp, which is §17.3's rule: one
							     identifies the moment, the other answers "how old is this?"
							     without arithmetic. The separator is written as an entity
							     because Svelte collapses the whitespace either side of a
							     block, and `Prowlarr"— read at` is what that costs. -->
							{activeService.message}{#if read.absolute}&nbsp;— read at {read.absolute}, {read.relative}{/if}
						</p>
					{/if}
				{:else if catalogError}
					<!-- A genuine transport or 5xx failure, which is NOT one of the four
					     states the endpoint reports on a 200. The picker says so and falls
					     back to whatever searches have already named. -->
					<p class="picker__note">
						UsArr could not read its own indexer list, so this picker is showing only the indexers
						that searches have named so far.
					</p>
					<p class="verbatim">{catalogError}</p>
				{:else if catalog !== undefined}
					<!--
						⚠️ THE HONEST NEGATIVE, AND IT IS ONLY SAID ONCE THERE IS AN ANSWER.
						All four of the endpoint's states arrive on a 200 and the branch is on
						`status`, never on the code. The sentence and the one action that
						changes it are the server's own — it knows which of "nothing is
						configured", "never read", "answered none" and "you turned it off"
						this install is in, and they need four different actions.
					-->
					<p class="picker__note">{catalog.message}</p>
					{#if catalog.action}
						{#if catalogFixInServices}
							<p class="picker__note">
								<a class="btn btn--sm" href={servicesPath}>{catalog.action}</a>
							</p>
						{:else}
							<!-- §17.5: naming the non-action beats offering a fake one. This is
							     the `empty` state, whose fix is inside the indexer service —
							     UsArr has no surface for it and Services would correctly show
							     the connection as healthy. -->
							<p class="picker__note muted">
								{catalog.action}. UsArr has no screen for that; it happens in the indexer service
								itself.
							</p>
						{/if}
					{/if}
					{#if scope.known.length > 0}
						<p class="picker__note muted">
							The indexers below are the ones searches have named, kept so a remembered selection
							can still be read and cleared.
						</p>
					{/if}
				{/if}

				{#if !usingCatalogue && scope.known.length > 0}
					<!-- The `never_fetched` fallback, and it is the ACTIVE service's
					     learned names only — `scope.known` filters on the instance the
					     frames were filed under. An indexer id learned from one Prowlarr
					     names nothing in another, so a merged list here would offer the
					     same false choice the flat catalogue grid used to. Keyed on the
					     pair for the same reason the catalogue grid is. -->
					<div class="picker__grid">
						{#each scope.known as indexer (`${indexer.instanceId}:${indexer.id}`)}
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
			</fieldset>
		</div>
	{/if}

	{#if categoryPickerOpen}
		<div class="picker" id="category-picker">
			<fieldset class="picker__set">
				<legend class="picker__legend">Categories to search</legend>

				{#if tree.length > 0}
					<!--
						⚠️ THE NEWZNAB TREE'S OWN TWO LEVELS, AND NEITHER OF THE TWO WAYS TO
						GET THIS WRONG. Flattening it hands the user a couple of hundred raw
						ids, which is not a control. Collapsing it into invented buckets
						throws away the leaves that are the only reliable machine signal
						there is — 3030 under Audio is what separates an audiobook from
						music, 7030 likewise for comics (§8.5). So the parents are the short
						path and each one opens on its own children, with every name taken
						verbatim from the tree.

						A parent id is worth offering on its own: the server matches a
						requested category against an indexer's advertised tree in both
						directions (releases/search.go, supportsAnyCategory), so `2000`
						reaches an indexer that only advertises `2045`.
					-->
					<ul class="cats">
						{#each tree as parent (parent.id)}
							{@const open = expandedCategories.includes(parent.id)}
							<li class="cats__row">
								<div class="cats__head">
									<label class="check">
										<input
											type="checkbox"
											checked={scope.isCategorySelected(parent.id)}
											onchange={() => toggleCategory(parent.id)}
										/>
										<span>{categoryLabelFor(parent)}</span>
									</label>
									{#if parent.children.length > 0}
										<button
											type="button"
											class="linkish"
											aria-expanded={open}
											aria-controls="cat-{parent.id}"
											onclick={() => toggleExpanded(parent.id)}
										>
											{open ? 'Hide' : 'Show'}
											{parent.children.length} inside
										</button>
									{/if}
								</div>
								{#if open}
									<div class="picker__grid cats__kids" id="cat-{parent.id}">
										{#each parent.children as child (child.id)}
											<label class="check">
												<input
													type="checkbox"
													checked={scope.isCategorySelected(child.id)}
													onchange={() => toggleCategory(child.id)}
												/>
												<span>{categoryLabelFor(child)}</span>
											</label>
										{/each}
									</div>
								{/if}
							</li>
						{/each}
					</ul>

					<p class="picker__note muted">
						<!-- "the indexers this search would ask" rather than "your indexers":
						     the tree is built from the ACTIVE service's indexers, so with a
						     second service configured the possessive would claim a coverage
						     the tree does not have. -->
						These are the categories advertised by the indexers this search would ask, in their own words.
						An indexer that carries none of the ones you pick is skipped before it is asked. Selecting
						none searches every category.
						{#if !scope.isAll}
							This list is narrowed to the indexers you selected.
						{/if}
					</p>
				{:else if usingCatalogue}
					<!-- The catalogue is here and still yields no tree, which is a real
					     state rather than a gap: either the selected indexers advertise no
					     categories, or nothing selected can be searched. -->
					<p class="picker__note">
						{scope.isAll
							? 'None of the indexers this search would ask advertises a category list, so there is nothing to narrow a search to. A search still runs; it simply cannot be scoped this way.'
							: 'The indexers you selected advertise no category list, so there is nothing to narrow this search to.'}
					</p>
					{#if !scope.isAll}
						<p class="picker__note">
							<button type="button" class="linkish" onclick={clearScope}>
								{clearLabel}
							</button>
						</p>
					{/if}
				{:else}
					<!-- No catalogue means no tree: the categories come from the same
					     replicated indexer list, and a search frame never carries one. The
					     indexer panel above says which of the four states this install is
					     in and what changes it. -->
					<p class="picker__note">
						Categories come from your indexer service’s own list, which UsArr has not read yet. Open
						Indexers above for what is missing and how to fix it.
					</p>
				{/if}
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

{#if diagnosis}
	<!--
		⚠️ ONE DIAGNOSIS, REPLACING N COPIES OF ONE SENTENCE — §17.5.

		The per-indexer banner below is right when three of nine indexers are in
		three different kinds of trouble, and wrong when all nine are in one: it
		renders the same title, the same explanation and the same reason nine
		times, and the fact that actually matters — that these are one condition
		rather than nine — appears nowhere. `correlatedFailure` decides, off the
		server's own `status` field rather than off its prose, and refuses to fire
		on anything short of every leg of a finished fan-out landing the same way.

		WHAT IS NOT CLAIMED HERE. UsArr sees its own half of the path and infers
		nothing about the far half, so the copy says what was observed and how
		much it supports — never a named cause the evidence merely permits.
		§17.5's own example is a resolution failure inside Prowlarr's container;
		it is one of several conditions that produce exactly this evidence, so it
		is not printed as the verdict.

		AND WHERE THE ANSWER IS "NOT FROM THIS SCREEN", IT SAYS SO. `nonAction` is
		the sentence naming what UsArr cannot do, and it is rendered instead of a
		button that would look like it worked. Only the class that HAS a working
		action carries one.
	-->
	<div
		class="banner"
		class:banner--err={diagnosis.tone === 'err'}
		class:banner--warn={diagnosis.tone === 'warn'}
		role="status"
	>
		<div class="banner__body">
			<div class="banner__title">{diagnosis.title}</div>
			<div class="banner__text">{diagnosis.text}</div>
			{#if diagnosis.nonAction}
				<div class="banner__text">{diagnosis.nonAction}</div>
			{/if}
			{#if diagnosis.verbatim}
				<!-- One shared sentence, so it is Prowlarr's own words ONCE. -->
				<p class="verbatim">{diagnosis.verbatim}</p>
			{:else}
				<!--
					The legs agreed on the status and not on the words, which is what a
					blocked or breaker-open set looks like: each line carries its own
					time, so dropping them would drop real data. They appear once each,
					as lines in ONE pre-wrapped block, rather than as N whole banners —
					what §17.5 objects to is the repeated explanation around them, not
					the upstream's own sentences.
				-->
				<p class="verbatim">{legReasons(report?.indexers ?? [])}</p>
			{/if}
		</div>
		{#if diagnosis.action !== 'none'}
			<div class="banner__actions">
				{#if diagnosis.action === 'clear-scope'}
					<!-- The label states what the control does, which now includes the
					     category half: `clearScope` clears both, because the scope
					     sentence above states both and a button that undid half of it
					     would look as though it had not worked. -->
					<button type="button" class="btn btn--primary" onclick={clearScope}>
						{clearLabel}
					</button>
				{:else}
					<button type="button" class="btn" onclick={() => runSearch()}>Search again</button>
				{/if}
			</div>
		{/if}
	</div>
{:else}
	{#each problems as problem (problem.indexer)}
		<!--
			One banner per indexer that did not answer, non-modal, with the upstream's
			own words verbatim (§9.5, §17.3). Per-indexer rather than one aggregate
			line: "3 indexers are not answering" is not something a user can act on,
			and the three reasons are usually three different problems — which is
			exactly the condition the branch above establishes is NOT the case before
			it collapses them.
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
{/if}

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
		emptyTitle={emptyCopy.title}
		emptyText={emptyCopy.text}
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
				<!-- A figure and its unit are two slots, not one string — §9.1. Right-
				     aligning `4.8 GiB` over `820 MiB` as one string aligns the `B`, so
				     the digits — the one thing compared down this column — land at two
				     x-positions, and the cell's `tabular-nums` cannot help because it
				     is the WORD that moves them. The reserved box is the half that
				     does the work.

				     ⚠️ 3ch, not §9.1's 2.5ch, and that is its own rule applied rather
				     than disagreed with: §9.1 reserves the widest unit the column can
				     ever print and derives 2.5ch from the DECIMAL family it measured,
				     while this column prints BINARY units because that is what every
				     indexer reports. `MiB` is the wider word — see `.unit--size` in
				     app.css, which owns the number.

				     The `{:else}` arm is why `sizeParts` returns null rather than an
				     empty pair: an absent size gets §9.1's em dash, not a 3ch reserve
				     held open around nothing. -->
				{@const size = sizeParts(release.sizeBytes)}
				{#if size}
					{size.value} <span class="unit unit--size">{size.unit}</span>
				{:else}
					{NOTHING.empty}
				{/if}
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
					{#if grab.releaseTitle}
						<span class="mono trunc" title={grab.releaseTitle}>{grab.releaseTitle}</span>
					{:else}
						<!--
							⚠️ AN EMPTY TITLE IS A FACT HERE, NOT MISSING DATA, so it gets a
							sentence rather than `NOTHING.empty`'s em dash. The candidate had
							already been swept when the audit row was written — `release_
							candidate` is the only place a release name ever lives and it goes
							25 minutes after the search — so UsArr genuinely does not know
							which release this was. An em dash would render that as an
							unremarkable blank; a guessed name would be worse still, since
							recognising the release is the entire job of this column.
						-->
						<span class="muted">{GRAB_MISSING_TITLE_NOTE}</span>
					{/if}
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
					<!-- A figure and its unit are two slots, not one string — §9.1, the
					     same treatment the release table's Size carries. The reserved box
					     is the half that does the work: right-aligning `4.8 GiB` over
					     `820 MiB` as one string aligns the `B`, so the digits land at two
					     x-positions, and `tabular-nums` cannot help because it is the WORD
					     that moves them. §9.1's exclusion list names `Age` on the two
					     release tables and `Items` on Home; a size column is on neither.

					     ⚠️ 3ch, not §9.1's 2.5ch — §9.1's own rule applied rather than its
					     number copied, because it derives 2.5ch from the DECIMAL family
					     and this column prints BINARY units. `.unit--size` in app.css owns
					     the number.

					     ⚠️ THE `{:else}` ARM IS REACHABLE HERE, unlike on the release
					     table. `internal/httpapi/grabs.go` tags `SizeBytes *int64` with
					     `omitempty`, so a not-sent row genuinely sends no size — where the
					     release table's field has no `omitempty` and can only ever send 0.
					     This is real behaviour, not a defensive branch, which is why
					     `sizeParts` returning null rather than an empty pair matters: the
					     em dash gets no `.unit`, so no 3ch is held open around nothing.
					     Not every indexer reports a size, and 0 would be a lie rather than
					     an absence. -->
					{@const size = sizeParts(grab.sizeBytes)}
					{#if size}
						{size.value} <span class="unit unit--size">{size.unit}</span>
					{:else}
						<span class="muted">{NOTHING.empty}</span>
					{/if}
				{:else if column.id === 'outcome'}
					{@const outcome = grabOutcome(grab.outcome, {
						errorCode: grab.errorCode,
						hasTitle: grab.releaseTitle !== ''
					})}
					<!--
						THREE STATES, AND THE GROUPING IS THE WHOLE RULE. The two
						handed-over ones both read "sent" and sit beside each other: they
						are the SAME epistemic state about the download, differing only in
						whether an error came back after the handoff. The third reads "not
						sent" and sits beside neither, because nothing was handed over and
						nothing can be running — it is the only state §17.5 permits to be
						worded as "it did not happen".

						Tone follows §9.5's "chroma marks what is wrong, not what is fine":
						neutral on the ordinary sent row, warn where the outcome is unknown
						and worth checking, err where the release never left. The labels
						differ as well as the tone, so removing the colour still leaves all
						three legible.

						There is no Retry on any row, on any state, permanently. And no
						handed-over row offers ANY action: the only control that would fit
						is one that sends the release again, which is exactly what produces
						two copies of a 68 GB release.

						THE SUB-LINES ARE CONDITIONAL, and that is §9.1 rather than a
						nicety: a clause identical on every row of a state is not data,
						and "the client accepted it" under every `sent` chip restated
						the chip at the cost of a second line on every confirmed row.
						$lib/requests decides which state carries what; this renders what
						it is given, and renders nothing when it is given nothing.
					-->
					<span
						class="chip"
						class:chip--pending={outcome.tone === 'warn'}
						class:chip--err={outcome.tone === 'err'}>{outcome.label}</span
					>
					{#if outcome.detail}
						<div class="cell-sub">{outcome.detail}</div>
					{/if}
					{#if outcome.nonAction}
						<!-- §17.5: naming the non-action beats offering a fake one. This
						     line is present exactly where the row's condition is not one
						     the offered control resolves — and on most codes there is no
						     control at all, which is what it says. -->
						<div class="cell-sub">{outcome.nonAction}</div>
					{/if}
					{#if outcome.action === 'search-again'}
						<!--
							A LINK, NOT A BUTTON, and the href is real: `?q=` is the
							parameter this screen already seeds a search from, so a
							middle-click opens a tab that runs the search by itself. A plain
							click is intercepted and runs it in place.

							`Search again` rather than `Retry`, and the distinction is not
							cosmetic: this starts a fresh fan-out and posts nothing. Nothing
							on this block re-sends a grab.

							`no-navigation-without-resolve` wants the href to BE a `resolve()`
							call, and a `ResolvedPathname` cannot carry a query string — the
							same limitation `writeUrl` documents above. `searchAgainHref` is
							built from `resolve('/requests')` with `?q=` appended, so this is
							a resolved path plus the one parameter that makes the link work,
							and the rule is suppressed at the single call site rather than
							switched off for the file.
						-->
						{@const href = searchAgainHref(grab.releaseTitle)}
						<div class="cell-sub">
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a resolve()'d path plus ?q=, which a ResolvedPathname cannot carry; see writeUrl -->
							<a {href} class="btn btn--sm" onclick={(e) => searchAgainFrom(e, grab)}>
								Search again
							</a>
						</div>
					{:else if outcome.action === 'open-services'}
						<!-- Offered only where UsArr's OWN service configuration is what
						     stopped the grab. §17.5 warns that Services is a dead end when
						     the fault is past Prowlarr — it will correctly show green — so
						     it is not offered on the codes that describe Prowlarr's
						     settings or an indexer's answer. -->
						<div class="cell-sub"><a class="btn btn--sm" href={servicesPath}>Open Services</a></div>
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

	/* The service radios sit in their own box above the indexer box, so the
	 * boundary between "which service" and "which of its indexers" is a thing
	 * you can see rather than a thing you infer from a legend. Compact: it is one
	 * short list, and on a phone it must not push the indexers off the screen. */
	.picker__set--services {
		margin-bottom: var(--space-3);
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

	/* One indexer in the picker: a name over as many facts as the catalogue has
	 * for it. `.check` centres a single-line label, which is wrong the moment
	 * there are two lines — the box would float against the middle of the block
	 * rather than lining up with the name it belongs to. */
	.pick {
		align-items: start;
	}

	.pick input {
		/* The 14px box inside a --text-base line, nudged onto the name's baseline
		 * band rather than the top of it. */
		margin-top: var(--space-1);
	}

	.pick__body {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-width: 0;
	}

	.pick__name {
		overflow-wrap: anywhere;
	}

	.pick__sub {
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	/* An indexer a search cannot ask is dimmed and still legible — it is listed
	 * precisely so somebody hunting for it finds it. No chroma: §9.5 reserves
	 * that for what is wrong, and an indexer the user switched off is not wrong. */
	.pick--off .pick__name {
		color: var(--fg-muted);
	}

	.pick__sub--off {
		color: var(--fg-muted);
	}

	/* The category tree's two levels. A list rather than a grid at the top level:
	 * each parent owns a disclosure and, when open, a block of children, so the
	 * rows are not the same height and column flow would interleave them. */
	.cats {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.cats__head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2) var(--space-4);
	}

	/* Indented under its parent, and the rule is what carries the nesting when
	 * the indent alone is ambiguous on a narrow screen. */
	.cats__kids {
		margin: var(--space-2) 0 var(--space-3) var(--space-5);
		padding-left: var(--space-4);
		border-left: 1px solid var(--border);
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
