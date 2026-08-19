<script lang="ts">
	/**
	 * LIBRARY — every media type at once, over `GET /api/v1/library`.
	 *
	 * WHAT THIS SCREEN IS. §17.2's catalogue with no type filter on it: ONE
	 * unified table across all six media types, in one of the orders §7.6 serves,
	 * inside §7.3's `?lib=` library scope, keyset-paged to the end of the
	 * catalogue rather than to the first screenful. The six per-type screens are
	 * the same read with `media_type` set; this is the one where it is omitted.
	 *
	 * ⚠️ IT USED TO READ `GET /api/v1/library/recent` AND NO LONGER DOES, AND THE
	 * SWITCH IS THE POINT OF THIS FILE. That endpoint parses `limit` and `cursor`
	 * and nothing else, and its order is hard-coded `added_at DESC, id DESC`, so
	 * a sort control or a scope over it could only ever have operated on the
	 * keyset PREFIX in the DOM while presenting itself as operating on the
	 * library. This file's header used to argue exactly that, at length, as the
	 * reason both controls were absent. The argument was right and it has
	 * expired: the controls are here because they are applied SERVER-SIDE now,
	 * over the whole table, by an endpoint built to take them.
	 *
	 * ⚠️ WHAT THE SWITCH DID NOT CHANGE, MEASURED RATHER THAN ASSUMED. At the
	 * default — no `lib`, no `media_type`, `sort=added_at` — the browse read is
	 * the recent read row for row and order for order: the same six `work.kind`
	 * values, the same `added_at DESC, id DESC` off `ix_work_added`, the same
	 * two-statement handoff into the undated tail, the same scope predicate and
	 * the same row on the wire. `TestBrowseWorksUnfilteredIsBlockCsCorpus`
	 * (`internal/store/browse_test.go`) walks every page of the unfiltered browse
	 * and asserts it equals Block C's own order, undated rows included. So no row
	 * disappeared and none reordered, and this screen did not quietly become a
	 * different list under the same address.
	 *
	 * ⚠️ HOME'S BLOCK C KEEPS THE OTHER ENDPOINT, DELIBERATELY. §17.2 as amended
	 * by ADR-0028 closes Block C at ONE table, ONE order and NO filters — a sixth
	 * media type adds rows to it rather than a sixth region — and http-api.md §7
	 * is explicit that the browse read *"is a different endpoint from §1, not a
	 * superset of it"*. Home is that block and is unchanged. The two share a row
	 * shape and a paging rule and share NO cursor (§7.5).
	 *
	 * ⚠️ A TO Z IS NOT ON THE SORT CONTROL, AND ITS ABSENCE IS STATED. `sort_title`
	 * walks `ix_work_kind_sort`, which is `(kind, sort_title, id)`, and SQLite
	 * cannot supply `ORDER BY` from an index whose leading column is constrained
	 * by `IN` — so the order needs exactly ONE `work.kind`, and a view over every
	 * media type is six. `browseSortsFor` derives that from the kind COUNT, which
	 * is the store's own `len(kinds) != 1`, and `browseSortNote` is the sentence
	 * that says so. Neither is keyed on the library scope: a scope narrows rows
	 * and changes no index. The server's own 400 text never reaches this screen —
	 * it is one shared sentence for two refusals and it talks about `media_type`,
	 * about music and about `year` to a reader who asked about none of them.
	 *
	 * COVERS ARE ABSENT AND THAT HAS NOT CHANGED: there is no image endpoint
	 * anywhere in `internal/httpapi/server.go`'s route table, so a poster view
	 * would have nothing to draw.
	 *
	 * A LOCAL SQLITE READ, so principle 1 holds all the way through: one statement
	 * per page plus at most one small statement to resolve `?lib=` slugs, no *Arr,
	 * no metadata provider, no image fetch. §7.2's Tier 0 rule follows from that
	 * and is why nothing is drawn while a page is in flight: no skeleton, no
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
	 * WHERE THE LOGIC LIVES. The sort vocabulary, the `?lib=` rules, the query
	 * builder, the cursor-reset rule, the paging stop rule and the empty-state
	 * copy are all `$lib/librarygrid`'s, and the row parsing and availability
	 * rendering are `$lib/library`'s. `vitest.config.ts` is `environment: 'node'`
	 * with no Svelte plugin, so a rule inside an `{#if}` in this file is a rule
	 * nothing can test.
	 */
	import { onMount, untrack } from 'svelte';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import { pushState } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { ApiError, fetchServicesHealth } from '$lib/api';
	import HaveCell from '$lib/HaveCell.svelte';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import { homeMode, type HomeMode } from '$lib/home';
	import { LOAD_MORE_PAGE_SIZE, NOTHING, type ListColumn } from '$lib/list';
	import { cursorRejected, mediaTypeLabel, type RecentItem } from '$lib/library';
	import {
		appendBrowsePage,
		browseAllTypesRoute,
		browseEmptyState,
		browseFeedFor,
		browseHasMore,
		browseSortLabel,
		browseSortNote,
		browseSortsFor,
		emptyBrowseFeed,
		fetchBrowsePage,
		MAX_LIBRARY_SLUGS,
		nextBrowsePage,
		sameBrowseQuery,
		type BrowseFeed,
		type BrowseQuery
	} from '$lib/librarygrid';
	import { formatWhen } from '$lib/requests';

	/**
	 * BLOCK C'S FIVE COLUMNS, AND THE WIDTHS ARE ITS MEASURED WIDTHS RATHER THAN
	 * A SECOND SET. The same five slots hold the same five fields off the same
	 * wire shape, and two screens computing two layouts for one row is the drift
	 * ADR-0029 exists to prevent. Search's results table carries the identical
	 * five for the identical reason, plus a sixth of its own.
	 *
	 * ⚠️ THE `Type` COLUMN STAYS HERE AND IS DROPPED ON THE PER-TYPE SCREENS, and
	 * that is the same rule applied to two different views rather than an
	 * inconsistency. DESIGN-DIRECTION §9.1 says a column identical on every row is
	 * not data: on `/library/movies` the cell would print `Movies` for ever, and
	 * here it is the one field that varies most.
	 *
	 * ⚠️ EVERY TRACK IS FIXED OR `fr` WITH A ZERO FLOOR, AND NO `minmax()` HAS A
	 * CONTENT-SIZED TRACK IN EITHER POSITION. ADR-0029 makes every row its own
	 * grid, so `auto`, `max-content`, `min-content` or `fit-content()` resolves
	 * against its OWN row's contents and the header cannot agree with the body.
	 * `gridTemplate()`'s dev guard says so out loud on every render.
	 *
	 * NO COLUMN IS `sortable`. The orders this endpoint serves are not one per
	 * column — `popularity` has no column at all, and `added_at` has a fixed
	 * direction rather than a toggle — so the sort is the toolbar control
	 * DESIGN-DIRECTION §9.1a names beside the column header, and a header
	 * affordance that reached one of two orders in one direction would
	 * misdescribe what the server offers.
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
	const ROW_INTRINSIC_LIBRARY = 44;

	const servicesPath = resolve('/services');
	const librariesPath = resolve('/libraries');

	/**
	 * THE ADDRESS, RESOLVED. There is no `[type]` segment to validate here, so the
	 * only refusal this screen can make on its own is §7.3's 32-slug bound — and
	 * it is `browseAllTypesRoute`'s rather than this file's, because the bound is
	 * a REFUSAL and not a clamp: dropping slugs WIDENS the page, so a screen that
	 * resolved `?lib=` itself and forgot the bound would show more than was asked
	 * for and look correct doing it.
	 */
	const route = $derived(browseAllTypesRoute(page.url.searchParams));
	const query = $derived(route.k === 'ok' ? route.query : undefined);
	const sorts = $derived(query === undefined ? [] : browseSortsFor(query.mediaType));
	/** The one line that says why A to Z is not on the control above. UsArr's own
	 * words: see `browseSortNote` for why the server's 400 text is not used. */
	const sortNote = $derived(query === undefined ? undefined : browseSortNote(query));

	/**
	 * THE WHOLE PAGING POSITION, IN ONE VALUE: the rows read so far, the cursor
	 * for the next page, the page size the server said it applied, and the query
	 * they were all read under. `undefined` while the address names no servable
	 * query, which is a different state from "read nothing yet".
	 */
	let feed = $state<BrowseFeed | undefined>(undefined);
	let loading = $state(false);
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

	const more = $derived(feed !== undefined && browseHasMore(feed));
	const empty = $derived(query === undefined ? undefined : browseEmptyState(query, mode));

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
	const drawList = $derived(
		feed !== undefined && feed.loaded && (feed.items.length > 0 || modeRead)
	);

	/**
	 * ⚠️ THE CURSOR IS DROPPED WHENEVER THE ADDRESS CHANGES, AND NOTHING
	 * SERVER-SIDE WOULD CATCH IT IF IT WERE NOT.
	 *
	 * A cursor from this endpoint binds to `sort` ALONE. Replaying one under a
	 * different sort is a loud 400; replaying it under a different LIBRARY SCOPE
	 * is a `200 OK` whose page starts partway into a different corpus — rows
	 * skipped, nothing reported, no symptom. The reset rule is `browseFeedFor`'s
	 * and is tested there; this effect is the one place that applies it, and it
	 * applies it to the whole feed rather than to the cursor alone, because
	 * keeping the rows would show one scope's items under another scope's
	 * heading.
	 *
	 * `untrack` because the body WRITES `feed` and `loading`, which it also has
	 * to read: without it the write re-enters the effect. The dependency is the
	 * address and only the address.
	 */
	$effect(() => {
		const next = query;
		untrack(() => sync(next));
	});

	function sync(next: BrowseQuery | undefined) {
		if (next === undefined) {
			feed = undefined;
			return;
		}
		const current = feed === undefined ? emptyBrowseFeed(next) : browseFeedFor(feed, next);
		if (current !== feed) {
			feed = current;
			error = '';
			action = '';
			rejected = false;
		}
		if (!current.loaded && !loading) void loadPage();
	}

	/**
	 * ONE PAGE, AND THE STOP RULE IS NOT HERE.
	 *
	 * `nextBrowsePage` owns it: it answers `undefined` exactly when the server
	 * omitted `next_cursor`, and it asks for the page size the server ECHOED
	 * rather than the one this screen first asked for — the endpoint clamps at
	 * 200 silently, so a client that keeps sending its own number is paging
	 * against a size the server never agreed to. This function never looks at
	 * `items.length`: under `added_at` the server issues a second statement for
	 * the undated tail, so a short page is legal and is not the end of the list.
	 * A client that stopped on `items.length < limit` would truncate the table at
	 * exactly the row whose upstream reported no creation date.
	 *
	 * ⚠️ AND THE ANSWER IS DISCARDED IF THE ADDRESS MOVED WHILE IT WAS IN FLIGHT.
	 * A page read under one scope appended to a feed that now says another is the
	 * same corruption the cursor reset exists to prevent, arriving by the other
	 * door.
	 */
	async function loadPage() {
		const current = feed;
		if (current === undefined) return;
		const request = nextBrowsePage(current, LOAD_MORE_PAGE_SIZE);
		if (request === undefined) return;
		const asked = current.query;
		loading = true;
		try {
			const answer = await fetchBrowsePage(asked, request);
			if (feed === undefined || !sameBrowseQuery(feed.query, asked)) return;
			feed = appendBrowsePage(feed, answer);
			error = '';
			action = '';
			rejected = false;
		} catch (caught) {
			if (feed === undefined || !sameBrowseQuery(feed.query, asked)) return;
			error = caught instanceof ApiError ? caught.detail : String(caught);
			action = caught instanceof ApiError ? caught.action : '';
			// ⚠️ THE CURSOR THAT WAS SENT DECIDES THIS, NOT THE STATUS CODE. On this
			// endpoint `400 bad_request` also covers a bad sort, an unknown lib slug
			// and a malformed limit, so reading it as a stale bookmark would offer a
			// restart that fails identically. A request that carried no cursor cannot
			// have had one rejected.
			rejected = cursorRejected(caught, request.cursor);
		} finally {
			loading = false;
		}
	}

	/** Start again from the beginning of this order, which is the action the
	 * server names. It is a request with NO cursor, so it cannot fail the way the
	 * last one did. */
	function restart() {
		if (query === undefined) return;
		feed = emptyBrowseFeed(query);
		error = '';
		action = '';
		rejected = false;
		void loadPage();
	}

	/**
	 * THE SORT LIVES IN THE URL (DESIGN-DIRECTION §9.1a clause 6, ADR-0038), so a
	 * sorted view is linkable and survives a reload exactly rather than
	 * approximately. The key is the SERVER's own word — `added_at`, `popularity`,
	 * which is `library.default_sort`'s vocabulary verbatim — so the address and
	 * the wire cannot drift into two spellings of one state.
	 *
	 * ⚠️ THERE IS NO `&dir=` HERE, AND ITS ABSENCE IS DELIBERATE RATHER THAN AN
	 * OMISSION. `$lib/sortspec` pairs a key with a direction because it sorts rows
	 * the client already holds; this order is the server's, and each of its orders
	 * has exactly one direction (newest first, highest first). Writing a `dir` the
	 * endpoint does not accept would put a control in the address bar that changes
	 * nothing.
	 *
	 * `pushState` rather than `goto`: the screen is not going anywhere, so there
	 * is nothing to re-run, no scroll to reset and no focus to move off the
	 * control the user is still holding. It pushes rather than replaces, because
	 * Back should return to the order you were reading before.
	 */
	function onSort(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		// `SvelteURLSearchParams`, which is what `svelte/prefer-svelte-reactivity`
		// asks for and what routes/requests already uses for the same job. It is
		// not read reactively here; the rule is followed rather than argued with.
		const params = new SvelteURLSearchParams(page.url.search);
		params.set('sort', value);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry a query string; page.url.pathname is already resolved. Precedent: routes/search/+page.svelte's syncUrl.
		pushState(`${page.url.pathname}?${params.toString()}`, page.state);
	}

	/**
	 * The address with the library scope removed.
	 *
	 * ⚠️ THE PARAMETER IS DELETED, NEVER EMPTIED. `?lib=` with nothing in it is a
	 * `400`, not "no scope": the server tests PRESENCE, so `?lib=`, `?lib=%20`
	 * and `?lib=,,` are all refusals. An empty `media_type` is silently
	 * unfiltered and an empty `lib` is an error, which is the one asymmetry on
	 * this endpoint that is easy to get backwards.
	 */
	const clearScopeHref = $derived.by(() => {
		const params = new SvelteURLSearchParams(page.url.search);
		params.delete('lib');
		const rest = params.toString();
		return rest === '' ? page.url.pathname : `${page.url.pathname}?${rest}`;
	});

	/**
	 * The services read, and it is allowed to fail. Nothing on this screen is
	 * gated on it except the wording of an empty state, and `browseEmptyState`
	 * has an answer for exactly this case: it says the reason is unknown rather
	 * than asserting the library is empty on the strength of a read that did not
	 * answer.
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
		void loadMode();
		return () => clearInterval(tick);
	});
</script>

<svelte:head><title>Library · UsArr</title></svelte:head>

<!--
	NO <h2> RESTATING THE PAGE NAME. The shell's toolbar already renders it and
	the same string is the h1 inside main, so a heading here would put it on
	screen three times. Standing rule on every screen.
-->

{#if route.k === 'too-many-libraries'}
	<!--
		⚠️ A REFUSAL RATHER THAN A TRUNCATION, WHICH IS THE SERVER'S OWN RULE AND
		THE OPPOSITE OF `limit`'s. Dropping slugs WIDENS the page — fewer scope
		terms means more rows — so a client that quietly sent the first 32 would
		show a bigger library than the one that was asked for and look correct.
	-->
	<section class="section">
		<div class="empty">
			<h2 class="empty__title">Too many libraries in this address</h2>
			<p class="empty__text">
				This address scopes the view to {route.count} libraries and the limit is {MAX_LIBRARY_SLUGS}.
				Nothing was asked of the server: sending fewer would show more than was asked for, which is
				the one direction a scope must not fail in.
			</p>
			<div class="empty__actions">
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry a query string; page.url.pathname is already resolved -->
				<a class="btn btn--primary" href={clearScopeHref}>Show every library</a>
				<a class="btn" href={librariesPath}>Open Libraries</a>
			</div>
		</div>
	</section>
{:else if query !== undefined}
	<div class="pagehead">
		<p class="pagehead__meta">
			Everything UsArr has replicated from your services, across every media type. It renders from
			SQLite, so it never waits on a service.
		</p>
		{#if feed !== undefined && feed.loaded && feed.items.length > 0}
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

	<div class="toolbar">
		<label class="toolbar__label" for="library-sort">Sort</label>
		<!--
			⚠️ THE OPTIONS ARE WHAT THE SERVER CAN ACTUALLY SERVE FOR THIS CORPUS,
			AND A TO Z IS NOT AMONG THEM. `sort_title` walks `(kind, sort_title, id)`
			and SQLite cannot supply ORDER BY from an index whose leading column is
			constrained by `IN`, so the order needs a corpus of exactly ONE
			`work.kind` — and every media type at once is six. `browseSortsFor`
			derives this from the kind count, so it is the store's own
			`len(kinds) != 1` rather than a hard-coded exception, and it is NOT keyed
			on the library scope: a scope narrows rows and changes no index.
		-->
		<select id="library-sort" class="select" value={query.sort} onchange={onSort}>
			{#each sorts as sort (sort)}
				<option value={sort}>{browseSortLabel(sort)}</option>
			{/each}
		</select>
		{#if sortNote}
			<!--
				⚠️ STATED, NOT OFFERED-THEN-REFUSED, AND IN UsArr's OWN WORDS. The
				option is absent from the control above rather than present and
				answered with a banner, because the refusal is knowable before the
				request is sent. The sentence is `$lib/librarygrid`'s so a test can
				read it; the server's own 400 text is deliberately not used, being one
				shared sentence for two refusals that names a parameter, music and
				`year` to a reader who asked about none of them.
			-->
			<p class="toolbar__note toolbar__label">{sortNote}</p>
		{/if}

		{#if query.libraries.length > 0}
			<span class="toolbar__spacer"></span>
			<!--
				THE SCOPE NAMES ITS SLUGS RATHER THAN THE LIBRARIES' NAMES, because a
				name would need a second read and this screen has the slug in hand. The
				slug IS the library's durable URL identity (migration 0005 keeps it
				across a rename), so it is the honest label for a scope that lives in
				the address. The multi-select chip that WRITES this belongs above the
				nav (DESIGN-DIRECTION §8.1) and is not built; the address is honoured
				whether or not the control that sets it exists, and the Libraries screen
				links here with one slug on it.
			-->
			<span class="toolbar__label">
				Scoped to {query.libraries.join(', ')}
			</span>
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry a query string; page.url.pathname is already resolved -->
			<a class="btn btn--sm" href={clearScopeHref}>Show every library</a>
		{/if}
	</div>

	<section class="section">
		{#if error}
			<div class="banner banner--err" role="alert">
				<Icon name="x-circle" />
				<div class="banner__body">
					<div class="banner__title">Your library could not be read</div>
					<div class="banner__text">
						This is a local read from UsArr’s own database, so it failing is not an upstream
						problem. Nothing is missing here because an item did not arrive: the list simply could
						not be loaded.
					</div>
					<p class="verbatim">{error}</p>
					{#if action}<p class="banner__text">{action}</p>{/if}
					{#if rejected}
						<!--
							⚠️ THE SERVER'S OWN `action`, AND NO AUTOMATIC RETRY. A cursor this
							endpoint did not issue is a 400, deliberately, because a silent reset
							to page one turns a stale bookmark into a Load-more loop that re-serves
							the first page for ever. Sending the same cursor again would produce
							the same answer, so the fix is a restart the user presses, and it goes
							out with no cursor on it.
						-->
						<div class="empty__actions">
							<button type="button" class="btn btn--sm" onclick={restart}>
								Start again from the beginning of this list
							</button>
						</div>
					{/if}
				</div>
			</div>
		{/if}

		{#if drawList && feed !== undefined && empty !== undefined}
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
			-->
			<List
				label="Library"
				columns={COLUMNS}
				rows={feed.items}
				key={(item: RecentItem) => String(item.id)}
				total={more ? undefined : feed.items.length}
				rowIntrinsic={ROW_INTRINSIC_LIBRARY}
				stack="two-line"
				state={feed.items.length === 0 ? 'empty' : 'default'}
				emptyTitle={empty.title}
				emptyText={empty.text}
				emptyActions={servicesLink}
				hasMore={more}
				loadingMore={loading}
				onloadmore={loadPage}
				cell={libraryCell}
			/>
		{/if}
	</section>
{/if}

{#snippet servicesLink()}
	<!--
		The exit from every one of the empty states, and it is the same exit in all
		of them: what is connected is what decides whether this screen can ever have
		rows, and Services is the screen that owns that. §17.3 keeps the FIX on the
		row that owns it, so this is a link to that screen and never a second copy
		of the controls on it.
	-->
	<a class="btn btn--primary" href={servicesPath}>Open Services</a>
{/snippet}

{#snippet libraryCell(item: RecentItem, column: ListColumn)}
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
			state with one absent `created` field, and under `added_at` the server
			sorts such rows LAST rather than first, in a second statement, so that a
			missing date cannot claim the top of the newest-first order. Absolute and
			relative together, per §17.3: one identifies the moment, the other answers
			"how long ago" without arithmetic.
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
