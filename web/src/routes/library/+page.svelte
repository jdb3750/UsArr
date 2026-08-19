<script lang="ts">
	/**
	 * RECENTLY ADDED — the whole catalogue, over `GET /api/v1/library/recent`.
	 *
	 * WHAT THIS SCREEN IS. Home's Block C without Home's row limit: ONE unified
	 * table across every media type, newest first, keyset-paged to the end of the
	 * catalogue rather than to the first screenful. §17.2 as amended by ADR-0028
	 * fixes the shape of that table, and this screen renders the same five
	 * columns from the same wire type, because §6.2 makes those two responses
	 * identical key for key so that one row renders both.
	 *
	 * ⚠️ WHAT IT IS NOT, AND WHY NOT — because §16 asks for something wider and a
	 * reader will otherwise take this for a half-finished version of it. §16
	 * names a per-type library GRID, with covers, a media-type filter and a sort
	 * control. Every one of those is blocked on a backend read that does not
	 * exist today, and the blocks were checked against the handlers rather than
	 * assumed:
	 *
	 *   THE GRID and its per-type routes need a browse read. There is no
	 *     `GET /api/v1/library`; `internal/httpapi/server.go` routes
	 *     `/api/v1/library/recent` and nothing else under that prefix.
	 *   COVERS need an image endpoint. There is none, and `$lib/library`'s own
	 *     header names cover art as absent from this wire.
	 *   A TYPE FILTER needs a filtered read. `handleRecentWorks` parses `limit`
	 *     and `cursor`, and nothing else.
	 *   A SORT CONTROL needs a sortable read. The order is hard-coded
	 *     `added_at DESC, id DESC` in the statement itself.
	 *
	 * ⚠️ AND A CLIENT-SIDE FILTER OR SORT IS NOT THE WORKAROUND, WHICH IS THE
	 * ONE THING TO GET RIGHT HERE. What this screen holds is a keyset PREFIX of
	 * the catalogue: the newest N rows, for whatever N the user has pressed
	 * `Load more` up to. A control that filtered or sorted that prefix would
	 * present itself as operating on the library and would in fact operate on
	 * the prefix, so `no comics found` would mean `no comics in the newest 200
	 * rows` and a sort by title would put the alphabetically-first row of the
	 * prefix at the top and call it the library's first. Both are confidently
	 * wrong answers, which is worse than no control at all. The real controls
	 * arrive with the backend browse read that can apply them to the whole
	 * table; until then this screen offers neither, and no column is `sortable`.
	 *
	 * A LOCAL SQLITE READ, so principle 1 holds all the way through: no render
	 * path here waits on an *Arr or on a media server, and there is no upstream
	 * behind the endpoint to wait for. §7.2's Tier 0 rule follows from that and
	 * is why nothing is drawn while a page is in flight: no skeleton, no
	 * shimmer, no zeroed table.
	 *
	 * ⚠️ AND A DEGRADED SERVICE GREYS NOTHING HERE. §17.7 is explicit that the
	 * catalogue does not grey out when an instance is unreachable, because every
	 * row is a replica row that is exactly as true as it was before. The services
	 * read below therefore decides the WORDS of the empty state and nothing else;
	 * it can fail outright and the table still renders. The banner that names a
	 * broken instance belongs to the screen that owns the fix (§17.3 states a
	 * problem canonically once per screen: Services owns it, and Home's Block B
	 * surfaces it), so there is no second copy of it here.
	 *
	 * WHERE THE LOGIC LIVES. The paging position, the stop rule, the six-value
	 * type vocabulary and the whole of the availability rendering are
	 * `$lib/library`'s; which empty state is true is `$lib/libraryscreen`'s.
	 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a
	 * rule inside an `{#if}` in this file is a rule nothing can test.
	 */
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { ApiError, fetchServicesHealth } from '$lib/api';
	import HaveCell from '$lib/HaveCell.svelte';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import { homeMode, type HomeMode } from '$lib/home';
	import { LOAD_MORE_PAGE_SIZE, NOTHING, type ListColumn } from '$lib/list';
	import {
		appendPage,
		cursorRejected,
		EMPTY_RECENT_FEED,
		fetchRecentPage,
		hasMore,
		mediaTypeLabel,
		nextRequest,
		type RecentFeed,
		type RecentItem
	} from '$lib/library';
	import { recentEmptyState } from '$lib/libraryscreen';
	import { formatWhen } from '$lib/requests';

	/**
	 * BLOCK C'S FIVE COLUMNS, AND THE WIDTHS ARE ITS MEASURED WIDTHS RATHER THAN
	 * A SECOND SET. The same five slots hold the same five fields off the same
	 * wire shape, and two screens computing two layouts for one row is the drift
	 * ADR-0029 exists to prevent. Search's results table carries the identical
	 * five for the identical reason, plus a sixth of its own.
	 *
	 * ⚠️ EVERY TRACK IS FIXED OR `fr` WITH A ZERO FLOOR, AND NO `minmax()` HAS A
	 * CONTENT-SIZED TRACK IN EITHER POSITION. ADR-0029 makes every row its own
	 * grid, so `auto`, `max-content`, `min-content` or `fit-content()` resolves
	 * against its OWN row's contents and the header cannot agree with the body.
	 * `gridTemplate()`'s dev guard says so out loud on every render.
	 *
	 * NO COLUMN IS `sortable`, and the file header above is the reason: a header
	 * sort would sort the prefix and claim to have sorted the library.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'type', header: 'Type', width: 'minmax(0, 0.9fr)', stackLabel: false, stackLine: 2 },
		{ id: 'title', header: 'Title', width: 'minmax(0, 3.2fr)', stackLabel: false, stackLine: 1 },
		{ id: 'year', header: 'Year', width: 'minmax(0, 0.6fr)', align: 'end', stackLine: 'hidden' },
		{ id: 'have', header: 'Have', width: 'minmax(0, 1.7fr)', stackLine: 'hidden' },
		{ id: 'added', header: 'Added', width: '132px', stackLine: 2 }
	];

	/**
	 * A row carries a sub-line wherever the data has one (the relative time under
	 * the absolute one, a gap figure under a fraction), so it is Block C's
	 * two-line shape and takes Block C's measured figure. `ROW_INTRINSIC`'s
	 * default is measured on a ONE-line row and would be wrong by half, which
	 * shows as scroll-height jitter as `content-visibility` releases rows.
	 */
	const ROW_INTRINSIC_RECENT = 44;

	const servicesPath = resolve('/services');

	/**
	 * THE WHOLE PAGING POSITION, IN ONE VALUE. `$lib/library`'s `RecentFeed` is
	 * the rows read so far, the cursor for the next page and whether the server
	 * has answered at all. There is deliberately no `hasMore` boolean beside it:
	 * "is there more" is `cursor !== undefined` and nothing else, and two fields
	 * that must agree are two fields that can disagree.
	 */
	let feed = $state<RecentFeed>(EMPTY_RECENT_FEED);
	let loadingMore = $state(false);
	let error = $state('');
	/** The server's own `action`: the one thing it says fixes this. */
	let action = $state('');
	/** Whether the failure was the server rejecting a cursor UsArr sent it. That
	 * one is not retryable and gets a restart control rather than silence. */
	let rejected = $state(false);

	/**
	 * WHAT IS CONNECTED, WHICH DECIDES THE WORDS OF THE EMPTY STATE AND NOTHING
	 * ELSE. `modeRead` is separate from `mode` because an unanswered read and a
	 * failed one are the same value and opposite facts, and the empty state has
	 * different words for the failure than for any of the three modes.
	 */
	let mode = $state<HomeMode | undefined>(undefined);
	let modeRead = $state(false);

	let now = $state(new Date());

	const more = $derived(hasMore(feed));
	const empty = $derived(recentEmptyState(mode));

	/**
	 * WHETHER THERE IS ANYTHING TRUE TO DRAW YET.
	 *
	 * Rows are drawn the moment they arrive. An EMPTY answer waits for the
	 * services read as well, because the empty state's whole job is to say WHY
	 * it is empty and the three reasons read very differently: telling a user
	 * with no service at all that an import is on its way is exactly the
	 * invented status §17.7's state list exists to prevent. Both reads start
	 * together, so on the install that has rows this costs nothing, and on the
	 * install that has none it costs one local read.
	 */
	const drawList = $derived(feed.loaded && (feed.items.length > 0 || modeRead));

	/**
	 * ONE PAGE, AND THE STOP RULE IS NOT HERE.
	 *
	 * `nextRequest` owns it: it answers `undefined` when there is nothing left to
	 * ask for, which happens exactly when the server omitted `next_cursor`. This
	 * function never looks at `items.length`, so it cannot short-page itself into
	 * stopping early — and a short page is legal here, because `ListRecentWorks`
	 * issues a second statement for the undated tail at the one boundary where
	 * the dated range runs out. A client that stopped on `items.length < limit`
	 * would truncate the table at exactly the row whose upstream reported no
	 * creation date, silently and for ever.
	 */
	async function loadPage() {
		const request = nextRequest(feed, LOAD_MORE_PAGE_SIZE);
		if (request === undefined) return;
		loadingMore = true;
		try {
			feed = appendPage(feed, await fetchRecentPage(request));
			error = '';
			action = '';
			rejected = false;
		} catch (caught) {
			error = caught instanceof ApiError ? caught.detail : String(caught);
			action = caught instanceof ApiError ? caught.action : '';
			// ⚠️ A REJECTED CURSOR IS NOT RETRIED, AND THE SERVER'S OWN COMMENT IS
			// THE REASON: it answers 400 rather than silently resetting to page one,
			// because resetting "turns a stale bookmark into a Load-more loop that
			// re-serves the first page for ever and looks like the list is stuck".
			// Retrying here would build that loop on the other side of the wire.
			rejected = cursorRejected(caught);
		} finally {
			loadingMore = false;
		}
	}

	/** Start again from the newest items, which is the action the server names.
	 * It is a request with NO cursor, so it cannot fail the way the last one
	 * did. */
	function restart() {
		feed = EMPTY_RECENT_FEED;
		error = '';
		action = '';
		rejected = false;
		void loadPage();
	}

	/**
	 * The services read, and it is allowed to fail. Nothing on this screen is
	 * gated on it except the wording of an empty state, and `recentEmptyState`
	 * has a fourth answer for exactly this case: it says the reason is unknown
	 * rather than asserting the library is empty on the strength of a read that
	 * did not answer.
	 */
	async function loadMode() {
		try {
			mode = homeMode(await fetchServicesHealth());
		} catch {
			mode = undefined;
		} finally {
			modeRead = true;
		}
	}

	onMount(() => {
		// A minute, not a second: `formatRelative` is coarse by design, so a faster
		// tick re-renders every row to produce the same string.
		const tick = setInterval(() => (now = new Date()), 60_000);
		void loadPage();
		void loadMode();
		return () => clearInterval(tick);
	});
</script>

<svelte:head><title>Recently added · UsArr</title></svelte:head>

<!--
	NO <h2> RESTATING THE PAGE NAME. The shell's toolbar already renders it and
	the same string is the h1 inside main, so a heading here would put it on
	screen three times. Standing rule on every screen.
-->
<div class="pagehead">
	<p class="pagehead__meta">
		Everything UsArr has replicated from your services, newest first, across every media type. It
		renders from SQLite, so it never waits on a service.
	</p>
	{#if feed.loaded && feed.items.length > 0}
		<!--
			`so far` is the honest suffix while a cursor is outstanding: the endpoint
			is keyset-paginated and never sends a total, so the only number this
			screen has is how many rows it holds. Printing it bare would claim the
			library is that size.
		-->
		<p class="pagehead__meta">
			{feed.items.length}
			{feed.items.length === 1 ? 'item' : 'items'}{more ? ' so far' : ''}
		</p>
	{/if}
</div>

<section class="section">
	{#if error}
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">Your library could not be read</div>
				<div class="banner__text">
					This is a local read from UsArr’s own database, so it failing is not an upstream problem.
					Nothing is missing here because an item did not arrive: the list simply could not be
					loaded.
				</div>
				<p class="verbatim">{error}</p>
				{#if rejected}
					<!--
						⚠️ THE SERVER'S OWN `action`, AND NO AUTOMATIC RETRY. A cursor this
						endpoint did not issue is a 400, deliberately, because a silent reset
						to page one turns a stale bookmark into a Load-more loop that re-serves
						the first page for ever. Sending the same cursor again would produce
						the same answer, so the fix is a restart the user presses, and it goes
						out with no cursor on it.
					-->
					{#if action}<p class="banner__text">{action}</p>{/if}
					<div class="empty__actions">
						<button type="button" class="btn btn--sm" onclick={restart}>
							Start again from the newest items
						</button>
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if drawList}
		<!--
			`two-line` below 760 px, which §9.1 gives to a list that is SCANNED rather
			than read one record at a time.

			`total` IS PASSED ONLY ON THE LAST PAGE, and the omission is the honest
			answer rather than a gap. ARIA defines `aria-rowcount="-1"` for a total
			that is genuinely unknown, and it is unknown here by construction: a
			keyset endpoint never says how many rows exist. Passing `items.length`
			while a cursor is outstanding is what makes a screen reader say "row 3 of
			200" when the truth is "row 3 of 4,000", and it is what would put a "200
			of 200" count under a button that has thousands of rows left to fetch.

			NO FILTER CONTROL AND NO SORT CONTROL SITS ABOVE THIS TABLE. See the file
			header: both would operate on the keyset prefix in the DOM while
			presenting themselves as operating on the library. They arrive with the
			backend browse read that can apply them to the whole table.
		-->
		<List
			label="Recently added"
			columns={COLUMNS}
			rows={feed.items}
			key={(item: RecentItem) => String(item.id)}
			total={more ? undefined : feed.items.length}
			rowIntrinsic={ROW_INTRINSIC_RECENT}
			stack="two-line"
			state={feed.items.length === 0 ? 'empty' : 'default'}
			emptyTitle={empty.title}
			emptyText={empty.text}
			emptyActions={servicesLink}
			hasMore={more}
			{loadingMore}
			onloadmore={loadPage}
			cell={recentCell}
		/>
	{/if}
</section>

{#snippet servicesLink()}
	<!--
		The exit from every one of the four empty states, and it is the same exit
		in all four: what is connected is what decides whether this screen can ever
		have rows, and Services is the screen that owns that. §17.3 keeps the FIX
		on the row that owns it, so this is a link to that screen and never a
		second copy of the controls on it.
	-->
	<a class="btn btn--primary" href={servicesPath}>Open Services</a>
{/snippet}

{#snippet recentCell(item: RecentItem, column: ListColumn)}
	{#if column.id === 'type'}
		<!--
			§17.2's six-value navigation enum, RESOLVED SERVER-SIDE. This cell may
			never be derived from `item.kind`: the Tier 1 client index carries no
			format, so a browser holding `kind: 'book'` cannot tell an ebook from an
			audiobook, and deriving it here would silently collapse two of the six
			values into one. `$lib/library` owns the vocabulary and the fallback for
			a value outside it.
		-->
		<span class="trunc">{mediaTypeLabel(item.mediaType)}</span>
	{:else if column.id === 'title'}
		{#if item.title}
			<span class="cell-title trunc" title={item.title}>{item.title}</span>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{:else if column.id === 'year'}
		<!--
			⚠️ AN ABSENT YEAR RENDERS AS AN ABSENCE, NOT AS `0` AND NOT AS `1970`.
			The server omits the key rather than sending a zero, because a rendered
			zero is a claim about a release date.
		-->
		{#if item.year !== undefined}
			<span class="num">{item.year}</span>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{:else if column.id === 'have'}
		<!--
			§17.2's Have column, drawn by the component Home's Block C and the Search
			results table both draw it from. Every decision in it belongs to
			`$lib/library.haveCell` and to `$lib/HaveCell.svelte`: the tick that must
			never fire on `total: null`, the gap figure that carries §9.5's warn role,
			and the row nothing has counted, which carries no glyph at all. An inline
			copy of that chain here would be the third one, and the first two had
			already come apart in their styling before the component existed.
		-->
		<HaveCell {item} />
	{:else if column.id === 'added'}
		<!--
			⚠️ AN UNDATED ROW IS REAL AND RENDERS AS UNDATED. Kavita reaches that
			state with one absent `created` field, and the server sorts such rows LAST
			rather than first so that a missing date cannot claim the top of a table
			whose question is "what did I just get". Absolute and relative together,
			per §17.3: one identifies the moment, the other answers "how long ago"
			without arithmetic.
		-->
		{@const when = formatWhen(item.addedAt, now)}
		{#if when.absolute}
			<span class="num">{when.absolute}</span>
			<div class="cell-sub">{when.relative}</div>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{/if}
{/snippet}
