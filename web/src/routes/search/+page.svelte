<script lang="ts">
	/**
	 * SEARCH — ARCHITECTURE.md §17.4, over `GET /api/v1/search`.
	 *
	 * SEARCH OVER YOUR OWN LIBRARY, and the endpoint behind it is a TIER 0 LOCAL
	 * READ: `internal/httpapi/librarysearch.go` is two SQLite statements with no
	 * *Arr call, no metadata provider and no image fetch. Nothing on this screen
	 * waits for an upstream, nothing here opens an SSE stream, and there is
	 * therefore no loading state to draw — §10 is explicit that a Tier-0
	 * component has none and that inventing one is the failure.
	 *
	 * ⚠️ THIS SCREEN USED TO BE THE RELEASE-SEARCH SURFACE, AND THEN IT WAS A
	 * NOT-BUILT GAP NOTICE. Both are gone. The Prowlarr fan-out moved to Requests
	 * (§17.5) at `4a51bd4` and answers on `GET /api/v1/releases/search`; the gap
	 * notice was correct until `04a28a4`, which is the commit that added the
	 * query this screen was waiting on. The four modules the fan-out used —
	 * `$lib/frozenorder.svelte`, `$lib/rowstate.svelte`, `$lib/sortspec` and
	 * `$lib/indexerscope.svelte` — stayed with Requests and none of them is
	 * imported here: there is exactly one release-search surface in UsArr, and
	 * this is not it.
	 *
	 * WHAT §17.4 SPECIFIES THAT THIS COMMIT DOES NOT DRAW, NAMED RATHER THAN
	 * QUIETLY OMITTED — every one of them because the wire cannot carry it, and
	 * `docs/reference/http-api.md` §6.6 says so field by field:
	 *
	 *   GROUPS. §17.4's rules 1-5 are all about per-media-type groups: the
	 *     ordering by best hit, the `clamp(floor(40 / groups), 3, 10)` row
	 *     budget, the `Show all 34 movies →` links, the library column's
	 *     within-group variance test. §6.2 answers with ONE FLAT LIST and §6.6
	 *     states "No grouping" first among what it does not do. A client cannot
	 *     synthesise the groups honestly either: rule 2 orders them by their
	 *     best-scoring hit and §6.2 publishes NO SCORE, so a client-side
	 *     grouping would have to order by count, which rule 2 names as exactly
	 *     the wrong rule a user will otherwise infer.
	 *   THE "NOT IN YOUR LIBRARY" SECTION. §6.6: this endpoint only ever returns
	 *     things UsArr has. Finding what you do not have is the release search,
	 *     over a different corpus, on the SSE stream — which is rule 6's action
	 *     below, and rule 6 is the part of that specification this commit CAN
	 *     honour.
	 *   THE GROUPED CARD FOR A LINKED WORK. §6.6: derived from `work_relation`
	 *     edges, and nothing reads those edges yet. A film and its novelisation
	 *     come back as two rows and are drawn as two rows.
	 *   THE LIBRARY COLUMN. Nothing service-side is on this response (§6.2), so
	 *     there is no library value to render or to collapse into a header.
	 *   THE SORT CONTROL. Rule 2's `Sort rows within each group:` needs groups.
	 *     The list is ordered by relevance and §6.2 makes that order the
	 *     contract, so this screen sorts nothing and offers no control that
	 *     would imply it could.
	 *
	 * ⚠️ AND THE COPY MAY NOT IMPLY A RANKING QUALITY THE SERVER HAS NOT GOT.
	 * §6.6 is blunt about four missing signals — no popularity, no `title_idf`,
	 * no `in_library` boost, no typo tolerance beyond the trigram leg — and the
	 * first consequence is that "short generic titles will over-rank, and there
	 * is nothing on the wire to compensate with". So nothing on this screen says
	 * "best match", "most relevant" or anything else that grades the order. It
	 * says the list is ranked, which is true, and stops. It does not explain the
	 * algorithm either: the rule is do not claim what is not true, not narrate
	 * the internals.
	 *
	 * WHERE THE LOGIC LIVES. `$lib/search` holds every decision that can be
	 * pinned by a test — the request URL, the response shape, the four screen
	 * states, the capacity test, rule 6's incompleteness test and its category
	 * mapping — because `vitest.config.ts` is `environment: 'node'` with no
	 * Svelte plugin, so a rule inside an `{#if}` here is a rule nothing can
	 * test. What stays here is markup and the copy, and the copy stays here
	 * deliberately: `designrules.test.ts` rule 9 reads
	 * `web/src/routes/search/**` and nothing wider, so a string moved into
	 * `$lib` is a string §17.4 rule 7 stops being enforced against.
	 */
	import { onMount } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import HaveCell from '$lib/HaveCell.svelte';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import { ApiError } from '$lib/api';
	import { mediaTypeLabel } from '$lib/library';
	import { NOTHING, type ListColumn } from '$lib/list';
	import { formatWhen } from '$lib/requests';
	import {
		atCapacity,
		fetchSearch,
		indexerSearchQuery,
		isCurrent,
		isIncomplete,
		longestTokenLength,
		screenState,
		SEARCH_TOKEN_CAP,
		TRIGRAM_FLOOR,
		truncatedTokens,
		type SearchItem,
		type SearchResults
	} from '$lib/search';

	/**
	 * THE COLUMNS, AND THEY ARE BLOCK C's COLUMNS ON PURPOSE.
	 *
	 * §17.2 asks Home's recently-added table for "the same small-multiple row as
	 * search", and §6.2 makes the two wire shapes identical key for key so that
	 * "one row component renders both". The widths are therefore Block C's
	 * measured widths rather than a second set: the same five slots hold the
	 * same five fields, and two screens computing two layouts for one row is the
	 * drift ADR-0029 exists to prevent.
	 *
	 * ⚠️ EVERY TRACK IS FIXED OR `fr`, WHICH THE DEV GUARD IN `gridTemplate()`
	 * ENFORCES, AND NO `minmax()` HAS A CONTENT-SIZED TRACK IN EITHER POSITION.
	 * ADR-0029 makes every row its own grid, so a content-sized track resolves
	 * against its OWN row's contents and the header cannot agree with the body.
	 * `minmax(0, …fr)` is the form that has a zero floor rather than `auto`.
	 *
	 * `132px` on `Added` is not a new measurement: it is the width Block C and
	 * the recent-grabs table already carry for identical content, an absolute
	 * time with a relative one under it.
	 *
	 * `find` IS §17.4 RULE 6's COLUMN AND IT IS DATA RATHER THAN FURNITURE. Rule
	 * 5 refuses a column that cannot vary; this one varies by construction,
	 * because it is empty on every row that is not missing anything. It is
	 * declared last because §17.4's slot invariant is identity first and the
	 * verdict last, and the exit from a bad verdict belongs beside the verdict.
	 * `150px` is a reserve rather than a measurement: the label is one fixed
	 * string, so the track only has to hold it.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'type', header: 'Type', width: 'minmax(0, 0.9fr)', stackLabel: false, stackLine: 2 },
		{ id: 'title', header: 'Title', width: 'minmax(0, 3.2fr)', stackLabel: false, stackLine: 1 },
		{ id: 'year', header: 'Year', width: 'minmax(0, 0.6fr)', align: 'end', stackLine: 'hidden' },
		{ id: 'have', header: 'Have', width: 'minmax(0, 1.7fr)', stackLine: 'hidden' },
		{ id: 'added', header: 'Added', width: '132px', stackLine: 2 },
		{ id: 'find', header: 'Indexers', width: '150px', stackLabel: false, stackLine: 2 }
	];

	/**
	 * A row carries a sub-line wherever the data has one (the relative time under
	 * the absolute one, a gap figure under a fraction), so it is Block C's
	 * two-line shape and takes Block C's measured figure. `ROW_INTRINSIC`'s
	 * default is measured on a ONE-line row and would be wrong by half, which
	 * shows as scroll-height jitter.
	 */
	const ROW_INTRINSIC_RESULT = 44;

	const requestsPath = resolve('/requests');

	/** What is in the box. It is NOT what was searched: `results.query` is, and
	 * the two differ for as long as the user is typing. */
	let query = $state('');

	/** The last answer, or undefined for "never answered". See `screenState`. */
	let results = $state<SearchResults | undefined>(undefined);
	let failure = $state<{ message: string; action: string } | undefined>(undefined);

	/**
	 * THE IN-FLIGHT READ, SO A SECOND SEARCH CANCELS THE FIRST.
	 *
	 * Two reads can be outstanding at once and they can land out of order, at
	 * which point the older answer overwrites the newer one and the screen shows
	 * results for a query the user has already replaced. `isCurrent()` is the
	 * guard — it compares the server's echoed `query`, which §6.2 says is
	 * echoed for exactly this — and the sequence number below is the second
	 * half: two searches for the SAME text would both pass `isCurrent`.
	 */
	let inflight = 0;

	let now = $state(new Date());

	const view = $derived(screenState(results, failure));
	const truncated = $derived(results ? truncatedTokens(results) : 0);
	const capped = $derived(results !== undefined && atCapacity(results));

	/**
	 * The clock behind the relative timestamps, ticked once a minute.
	 *
	 * A minute, not a second: `formatRelative` is coarse by design, so a faster
	 * tick re-renders every row to produce the same string. The interval is
	 * cleared on teardown.
	 */
	onMount(() => {
		const tick = setInterval(() => (now = new Date()), 60_000);

		// A LINKED QUERY RUNS ITSELF. `?q=` is what Home's search box and every
		// `Search indexers` link already speak, so a search is linkable in both
		// directions rather than only out of this screen.
		const linked = (page.url.searchParams.get('q') ?? '').trim();
		if (linked !== '') {
			query = linked;
			void run();
		}

		return () => clearInterval(tick);
	});

	/**
	 * THE URL, WRITTEN SHALLOWLY.
	 *
	 * `replaceState` rather than `goto`: the screen is not going anywhere, it is
	 * recording the state it is already in, so there is no load to re-run and no
	 * scroll to reset. Replace rather than push, because a Back button that
	 * walks every keystroke is not what Back is for.
	 *
	 * eslint's `no-navigation-without-resolve` wants a `resolve()` call, whose
	 * type is a bare `ResolvedPathname` and cannot carry a query string. The
	 * query string is the entire point here, so the URL is built from
	 * `page.url`, which is already resolved, and the rule is suppressed at the
	 * one call site rather than turned off for the file. Requests set this
	 * precedent for `?sort=`.
	 */
	function syncUrl(text: string) {
		const href =
			text === '' ? page.url.pathname : `${page.url.pathname}?q=${encodeURIComponent(text)}`;
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- see above: a ResolvedPathname cannot carry ?q=
		replaceState(href, page.state);
	}

	/**
	 * ONE SEARCH.
	 *
	 * ⚠️ THE PREVIOUS ANSWER STAYS ON SCREEN WHILE THIS RUNS, and that is the
	 * Tier 0 rule rather than an oversight: clearing the list first would draw
	 * an empty state for the few milliseconds a local SQLite read takes, which
	 * is a flash of "nothing matches" that is not true. §10 forbids the loading
	 * state that would otherwise fill the gap, so the gap is not created.
	 */
	async function run(event?: SubmitEvent) {
		event?.preventDefault();
		const text = query.trim();
		syncUrl(text);
		if (text === '') {
			results = undefined;
			failure = undefined;
			return;
		}

		const seq = ++inflight;
		try {
			const answer = await fetchSearch(text);
			// Two guards, and they catch different races: the sequence number
			// discards an older read of a DIFFERENT query, and `isCurrent` discards
			// any read whose echoed text no longer matches the box.
			if (seq !== inflight || !isCurrent(answer, query)) return;
			results = answer;
			failure = undefined;
		} catch (error) {
			if (seq !== inflight) return;
			results = undefined;
			failure =
				error instanceof ApiError
					? { message: error.detail, action: error.action }
					: { message: String(error), action: '' };
		}
	}
</script>

<svelte:head><title>Search · UsArr</title></svelte:head>

<!--
	NO <h2> RESTATING THE PAGE NAME. The shell's toolbar already renders it and
	the same string is the h1 inside main, so a heading here would put it on
	screen three times. Standing rule on every screen.
-->

<div class="pagehead">
	<!--
		§17.4 rule 7's STANDING WORDING, and it is quoted from §17 rather than
		written here. The rule exists because this line used to enumerate media
		types, on an install where nothing could supply four of the ones it named.
		The set behind "your services" is whatever is connected, which is a
		different set on a Prowlarr-only install than on a full stack, so any fixed
		list is a promise the install may not keep. `designrules.test.ts` rule 9
		reads the sentence live out of ARCHITECTURE §17.4 and fails if this drifts
		from it.

		AND THERE IS NO SENTENCE HERE EXPLAINING THE SILENCE for a type no service
		supplies. §17.4 rule 7 refuses one: nothing in the schema or on the wire
		separates "no rows matched" from "no source can supply this", so no string
		may claim to.
	-->
	<p class="pagehead__meta">
		This screen searches the library UsArr has replicated from your services. It covers what those
		services supply and nothing else, and it renders from SQLite, so it never waits on one of them.
	</p>
</div>

<section class="section">
	<form class="searchbar" onsubmit={run}>
		<!--
			A LABEL, NOT A PLACEHOLDER-AS-LABEL. The placeholder is gone the moment
			the field is typed into, and the label is what a screen reader announces.
		-->
		<label class="sr" for="lib-query">Search terms</label>
		<input
			id="lib-query"
			name="q"
			type="search"
			class="searchfield searchbar__input"
			autocomplete="off"
			placeholder="A title, or a name credited on one"
			bind:value={query}
		/>
		<button type="submit" class="btn btn--primary" disabled={query.trim() === ''}>Search</button>
	</form>
</section>

{#if view.k === 'error'}
	<section class="section">
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">Your library could not be searched</div>
				<div class="banner__text">
					This is a read of UsArr’s own database and nothing else, so it failing is not a service
					being slow or unreachable. Your library is not known to be empty; it is unknown.
				</div>
				<p class="verbatim">{view.message}</p>
				{#if view.action}<p class="banner__text">{view.action}</p>{/if}
			</div>
		</div>
	</section>
{:else if view.k === 'idle'}
	<!--
		NO QUERY YET, AND IT IS NOT AN EMPTY RESULT. §9.6's empty state answers
		"you own nothing" or "a filter hid everything"; neither is true before a
		question has been asked, and drawing that block here would tell a user
		with a full library that they have none. One line of guidance, no box.
	-->
	<section class="section">
		<p class="hint">
			Type something above and press Search. A name credited on a work finds the work, so the author
			of a book is as good a query as its title.
		</p>
	</section>
{:else if view.k === 'empty'}
	<section class="section">
		<div class="empty">
			<!--
				§17.4's zero-results state: the query echoed back, the honest note about
				what the matching does and does not do, and the exit to §17.5.

				THE NOTE IS BOUNDED BY WHAT §6.6 ACTUALLY GUARANTEES. Prefix matching
				is the LAST token only (search.md §3 step 4), substring matching is the
				trigram leg over the whole query, and there is no typo tolerance beyond
				it. Nothing here promises a near miss will be found.
			-->
			<h2 class="empty__title">Nothing in your library matches “{view.query}”</h2>
			<p class="empty__text">
				The last word of a query is matched as a prefix, and the query as a whole is matched against
				any run of characters inside a title. Spelling is not corrected, so a word typed wrong finds
				nothing rather than something close.
			</p>
			{#if longestTokenLength(query) < TRIGRAM_FLOOR}
				<p class="empty__text">
					Every word here is shorter than {TRIGRAM_FLOOR} characters, and below that length only the prefix
					match runs. A longer word is matched inside a title as well as at the start of one.
				</p>
			{/if}
			<div class="empty__actions">
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry ?q= -->
				<a class="btn btn--primary" href={`${requestsPath}?q=${encodeURIComponent(query.trim())}`}>
					Search indexers for this instead
				</a>
			</div>
		</div>
	</section>
{:else}
	<section class="section">
		<div class="section__head">
			<h2>Results</h2>
			<span class="section__count">{view.results.items.length}</span>
		</div>

		{#if truncated > 0}
			<!--
				§6.4, AND THE ONE THING THIS FLAG DOES NOT MEAN IS THAT RESULTS WERE
				CUT. It says search.md §3 step 1's twelve-token cap fired, so the
				answer is for a PREFIX of what was typed. §6.4: "It says nothing about
				how many results came back." Copy of the form "there are more matches
				than shown" would be false on this field, and it is the obvious wrong
				reading, so the note names the words rather than the rows.
			-->
			<div class="banner banner--warn" role="status">
				<Icon name="alert" />
				<div class="banner__body">
					<div class="banner__title">Only part of your query was searched</div>
					<div class="banner__text">
						A search reads the first {SEARCH_TOKEN_CAP} words. You typed {truncated}, so the results
						below answer the first {SEARCH_TOKEN_CAP} of them and the rest were not used.
					</div>
				</div>
			</div>
		{/if}

		{#if capped}
			<!--
				§6.5: there is no cursor, no offset and no second page, because the
				store re-ranks the whole fused candidate set in Go, so the ordering is
				a property of that set and there is no keyset position a cursor could
				name. A full page therefore means the endpoint stopped, not that the
				library did, and saying nothing would be the silent truncation §17.4
				rule 3 names as a finding of Baymard's. It does NOT claim a number of
				hidden rows, because nothing on the wire carries one.
			-->
			<p class="hint">
				{view.results.limit} results is the most this screen shows at once, and there is no second page.
				If what you want is not here, a narrower query is how to reach it.
			</p>
		{/if}

		<!--
			ONE FLAT LIST, ORDERED BY RELEVANCE, AND THE ORDER IS THE CONTRACT (§6.2).
			No sort control: §6.2 publishes no score, and rule 2's sort is scoped to
			groups this wire has none of.

			`total` is the rendered count and that is the whole result set: §6.5 says
			a caller asks for up to the cap and that is every row the endpoint has,
			so unlike Home's keyset table there is no outstanding page to make
			`aria-rowcount` a lie.
		-->
		<List
			label="Search results"
			columns={COLUMNS}
			rows={view.results.items}
			key={(item: SearchItem) => String(item.id)}
			total={view.results.items.length}
			rowIntrinsic={ROW_INTRINSIC_RESULT}
			stack="two-line"
			cell={resultCell}
		/>
	</section>
{/if}

{#snippet resultCell(item: SearchItem, column: ListColumn)}
	{#if column.id === 'type'}
		<!--
			§17.2's six-value navigation enum, RESOLVED SERVER-SIDE, and this cell
			may never be derived from `item.kind`: both an ebook and an audiobook are
			`kind: 'book'` and only `edition.format` separates them, which is not on
			this wire. `$lib/library` owns the vocabulary and the fallback for a
			value outside the six.
		-->
		<span class="trunc">{mediaTypeLabel(item.mediaType)}</span>
	{:else if column.id === 'title'}
		{#if item.title}
			<span class="cell-title trunc" title={item.title}>{item.title}</span>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{:else if column.id === 'year'}
		<!-- An absent year renders as an absence, never as `0` and never as
		     `1970`: the server omits the key rather than sending a zero, because a
		     rendered zero is a claim about a release date. -->
		{#if item.year !== undefined}
			<span class="num">{item.year}</span>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{:else if column.id === 'have'}
		<!--
			§17.2's Have column, rendered by the same component Home's Block C uses.
			Every decision in it is `$lib/library.haveCell`'s and
			`$lib/HaveCell.svelte`'s, including the one `http-api.md` §1.4.1 states for
			a results table by name: an absent `availability` means no count has been
			computed, so a result row must carry no emptiness glyph and no accessible
			name like *none held* on the strength of a missing key.
		-->
		<HaveCell {item} />
	{:else if column.id === 'added'}
		<!-- An undated row is real and renders as undated: Kavita reaches that
		     state with one absent `created` field. Absolute and relative together
		     per §17.3, one identifying the moment and the other answering "how long
		     ago" without arithmetic. -->
		{@const when = formatWhen(item.addedAt, now)}
		{#if when.absolute}
			<span class="num">{when.absolute}</span>
			<div class="cell-sub">{when.relative}</div>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{:else if column.id === 'find'}
		<!--
			§17.4 RULE 6, AND IT IS THE ONLY EXIT v0.1 HAS FROM THE SCREEN WHERE A
			USER DEFINITIVELY LEARNS THEY ARE MISSING SOMETHING. It renders on every
			row whose availability is not a full tick, links to §17.5's free-text
			indexer search with the query taken from this row's title, and carries
			the Newznab category §8.5's table maps this media type to. A complete row
			gets nothing: rule 5 refuses a column that cannot vary, and this one
			varies because the verdict beside it does.
		-->
		{#if isIncomplete(item)}
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry ?q= -->
			<a class="btn btn--sm" href={`${requestsPath}?${indexerSearchQuery(item)}`}>Search indexers</a
			>
		{/if}
	{/if}
{/snippet}

<style>
	/* The search ROW — placement only. The field's appearance is app.css's
	   `.searchfield`, which the input carries as its first class; these are flex
	   participation, which no shared class may own. Requests' toolbar carries the
	   identical pair for the identical reason. */
	.searchbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}

	/* 24rem, matching Home rather than Requests: this row holds the field and one
	   button, where the Requests toolbar also carries a label, a select and two
	   more buttons. `min-width: 0` overrides the flex automatic minimum size,
	   which for an <input> is its `size`-derived intrinsic width and makes the
	   document scroll sideways below roughly a 209px viewport. */
	.searchbar__input {
		flex: 1 1 24rem;
		min-width: 0;
	}

	/* A line of guidance, not a banner: it carries no icon, no border and no
	   role, because none of the three states it appears in is a warning. */
	.hint {
		margin: 0 0 var(--space-4);
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}
</style>
