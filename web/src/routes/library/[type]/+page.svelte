<script lang="ts">
	/**
	 * ONE MEDIA TYPE — the per-type library grid, over `GET /api/v1/library`.
	 *
	 * WHAT THIS SCREEN IS. ARCHITECTURE.md §17.2's per-type view and §17.8's
	 * library scope, on the two-axis navigation model ADR-0027 settled and
	 * `docs/design/DESIGN-DIRECTION.md` §8.1 closed as OQ-2: MEDIA TYPE IS THE
	 * NAVIGATION AXIS, so `/library/movies` is a place with six siblings, and A
	 * LIBRARY IS A SCOPE, carried as `?lib=` and never a nav entry of its own.
	 * That is decided; this screen is not a filtered view over all six types
	 * wearing a type's name.
	 *
	 * ⚠️ IT IS THE TABLE HALF OF §16'S LINE ITEM AND NOT THE WHOLE OF IT. §16
	 * names a grid with COVERS. There is no image endpoint anywhere in
	 * `internal/httpapi/server.go`'s route table — checked, not assumed — so a
	 * poster view would have nothing to draw and `poster_asset_id` is not on this
	 * wire either (http-api.md §7.1 says why: it would be an id the client cannot
	 * turn into anything). This is the row grid; the poster view arrives with the
	 * endpoint that can feed it, and calling this line item finished would be the
	 * invented status CLAUDE.md forbids.
	 *
	 * ⚠️ AND IT IS NOT `/library`. That screen is THE SAME READ WITH NO
	 * `media_type` ON IT: every media type at once, sorted and scoped by the same
	 * endpoint, modelled by the same `$lib/librarygrid`. The difference is one
	 * optional field, which is why `BrowseQuery.mediaType` is optional and this
	 * screen takes the narrowed `TypedBrowseRoute` — its `[type]` segment is
	 * validated before anything is built, so its heading and its title never need
	 * a fallback for a case the route already refused.
	 *
	 * ⚠️ AND NEITHER OF THEM IS HOME'S BLOCK C, which reads `/library/recent` and
	 * keeps doing so: §17.2 as amended by ADR-0028 closes that block at one
	 * table, one order and no filters, and http-api.md §7 is explicit that the
	 * browse read *"is a different endpoint from §1, not a superset of it"*. The
	 * two share a row shape and a paging rule and share no cursor.
	 *
	 * A LOCAL SQLITE READ, so principle 1 holds all the way through: one
	 * statement per page plus at most one small statement to resolve `?lib=`
	 * slugs, no *Arr, no metadata provider, no image fetch. §7.2's Tier 0 rule
	 * follows from that and is why nothing is drawn while a page is in flight: no
	 * skeleton, no shimmer, no zeroed table.
	 *
	 * ⚠️ AND A DEGRADED SERVICE GREYS NOTHING HERE. §17.7 is explicit that the
	 * catalogue does not grey out when an instance is unreachable, because every
	 * row is a replica row that is exactly as true as it was before. The services
	 * read below decides the WORDS of the empty state and nothing else; it can
	 * fail outright and the table still renders.
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
	import { cursorRejected, MEDIA_TYPES, mediaTypeLabel, type RecentItem } from '$lib/library';
	import {
		appendBrowsePage,
		browseEmptyState,
		browseFeedFor,
		browseHasMore,
		browseRoute,
		browseSortLabel,
		browseSortNote,
		browseSortsFor,
		emptyBrowseFeed,
		fetchBrowsePage,
		libraryScopeLine,
		MAX_LIBRARY_SLUGS,
		nextBrowsePage,
		readLibraryNames,
		sameBrowseQuery,
		type BrowseFeed,
		type BrowseQuery,
		type LibraryNames
	} from '$lib/librarygrid';
	import { formatWhen } from '$lib/requests';

	/**
	 * FOUR COLUMNS, AND THE MISSING FIFTH IS THE DECISION.
	 *
	 * Block C's table carries Type · Title · Year · Have · Added, and this one
	 * drops Type. Every row on this screen is the type in the address bar — the
	 * filter is server-side and closed — so the cell would print the same word on
	 * every row for ever, and DESIGN-DIRECTION §9.1 is explicit that a column
	 * identical on every row is not data. The type is stated once, in the page
	 * head, where it is the heading rather than a repeated value.
	 *
	 * ⚠️ WHEREVER THE MEDIA TYPE IS RENDERED IT COMES FROM `media_type` AND NEVER
	 * FROM `kind`, AND THAT IS A CORRECTNESS RULE RATHER THAN A PREFERENCE. §17.2
	 * states the Tier 1 client index carries no format at all, so a browser
	 * holding `kind: 'book'` cannot tell an ebook from an audiobook — the split is
	 * `edition.format` and is resolved server-side. `$lib/library` owns the
	 * vocabulary and nothing here reads `kind`.
	 *
	 * ⚠️ EVERY TRACK IS FIXED OR `fr` WITH A ZERO FLOOR, AND NO `minmax()` HAS A
	 * CONTENT-SIZED TRACK IN EITHER POSITION. ADR-0029 makes every row its own
	 * grid, so `auto`, `max-content`, `min-content` or `fit-content()` resolves
	 * against its OWN row's contents and the header cannot agree with the body.
	 *
	 * NO COLUMN IS `sortable`. The three orders this endpoint serves are not one
	 * per column — `popularity` has no column at all, and `added_at` and
	 * `sort_title` each have a fixed direction rather than a toggle — so the sort
	 * is the toolbar control DESIGN-DIRECTION §9.1a names beside the column
	 * header, and a header affordance that could reach two of three orders in one
	 * direction each would misdescribe what the server offers.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'title', header: 'Title', width: 'minmax(0, 3.2fr)', stackLabel: false, stackLine: 1 },
		{ id: 'year', header: 'Year', width: 'minmax(0, 0.6fr)', align: 'end', stackLine: 2 },
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
	const ROW_INTRINSIC_GRID = 44;

	const servicesPath = resolve('/services');
	const librariesPath = resolve('/libraries');

	/**
	 * THE ADDRESS, RESOLVED. `browseRoute` validates the `[type]` segment against
	 * §17.2's six and bounds `?lib=` at 32 before anything is sent, so an address
	 * this endpoint would refuse never becomes a request: the answer to both is
	 * already known from http-api.md §7.2 and §7.3, and spending a round trip to
	 * be told it would put the server's wording on a mistake the URL bar made.
	 */
	const route = $derived(browseRoute(page.params.type ?? '', page.url.searchParams));
	const query = $derived(route.k === 'ok' ? route.query : undefined);
	const typeLabel = $derived(query === undefined ? '' : mediaTypeLabel(query.mediaType));
	const sorts = $derived(query === undefined ? [] : browseSortsFor(query.mediaType));
	/**
	 * The one line that says why A to Z is not on the control above, on the one
	 * type it is missing from. UsArr's own words, never the server's 400 text:
	 * `browseSortNote` decides from the kind count, so Music gets a sentence here
	 * for the same reason it gets two options rather than three.
	 */
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
	/** Whether the failure was the server rejecting a cursor UsArr sent it. */
	let rejected = $state(false);

	/**
	 * WHAT IS CONNECTED, WHICH DECIDES THE WORDS OF THE EMPTY STATE AND NOTHING
	 * ELSE. `modeRead` is separate from `mode` because an unanswered read and a
	 * failed one are the same value and opposite facts.
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
	 * services read as well, because the empty state's whole job is to say WHY it
	 * is empty and the reasons read very differently: telling a user with no
	 * service at all that this type is empty answers a question about the library
	 * with a fact about a filter.
	 */
	const drawList = $derived(
		feed !== undefined && feed.loaded && (feed.items.length > 0 || modeRead)
	);

	/**
	 * ⚠️ THE CURSOR IS DROPPED WHENEVER THE ADDRESS CHANGES, AND NOTHING
	 * SERVER-SIDE WOULD CATCH IT IF IT WERE NOT.
	 *
	 * A cursor from this endpoint binds to `sort` ALONE. Replaying one under a
	 * different sort is a loud 400; replaying it under a different MEDIA TYPE or
	 * a different LIBRARY SCOPE is a `200 OK` whose page starts partway into a
	 * different corpus — rows skipped, nothing reported, no symptom. The reset
	 * rule is `browseFeedFor`'s and is tested there; this effect is the one place
	 * that applies it, and it applies it to the whole feed rather than to the
	 * cursor alone, because keeping the rows would show one type's items under
	 * another type's heading.
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
	 *
	 * ⚠️ AND THE ANSWER IS DISCARDED IF THE ADDRESS MOVED WHILE IT WAS IN FLIGHT.
	 * A page read under `movies` appended to a feed that now says `comics` is the
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
			// endpoint `400 bad_request` also covers a bad media_type, a bad sort, an
			// unknown lib slug and a malformed limit, so reading it as a stale
			// bookmark would offer a restart that fails identically. A request that
			// carried no cursor cannot have had one rejected.
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
	 * approximately. The key is the SERVER's own word — `added_at`, `sort_title`,
	 * `popularity`, which is `library.default_sort`'s vocabulary verbatim — so
	 * the address and the wire cannot drift into two spellings of one state.
	 *
	 * ⚠️ THERE IS NO `&dir=` HERE, AND ITS ABSENCE IS DELIBERATE RATHER THAN AN
	 * OMISSION. `$lib/sortspec` pairs a key with a direction because it sorts
	 * rows the client already holds; this order is the server's, and each of its
	 * three has exactly one direction (newest first, A to Z, highest first).
	 * Writing a `dir` the endpoint does not accept would put a control in the
	 * address bar that changes nothing.
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
	 * THE LIBRARY NAMES, AND THE GRID IS NEVER GATED ON THEM.
	 *
	 * ⚠️ THE SCOPE LINE RENDERS BEFORE THIS ANSWERS AND RENDERS WHETHER OR NOT IT
	 * EVER DOES. `undefined` here is "no names yet", which `libraryScopeLine`
	 * spells as the slug — so the line states a true scope from the first frame and
	 * is REPLACED by a friendlier one, rather than waiting to be correct. Nothing
	 * else on this screen reads this value: the table, its empty state and its
	 * failure banner are all driven by the browse read alone, so a libraries read
	 * that is slow or dead costs one word and never a row (ARCHITECTURE §17.7).
	 *
	 * `readLibraryNames` cannot reject, so there is no catch here and nothing to
	 * report: a failure is indistinguishable from an install with no libraries, and
	 * neither is news the catalogue screen owes the user.
	 */
	let names = $state<LibraryNames | undefined>(undefined);
	/** One read per visit. Plain, not `$state`: it guards the read and must never
	 * be a reason to re-render. */
	let namesAsked = false;

	/**
	 * ⚠️ ASKED ONLY WHEN THE ADDRESS CARRIES A SCOPE, so the unscoped view — which
	 * renders no scope line at all — costs no read. The effect rather than
	 * `onMount` because the scope can arrive after mount: `pushState` moves the
	 * address without remounting this component.
	 */
	$effect(() => {
		if ((query?.libraries.length ?? 0) === 0 || namesAsked) return;
		namesAsked = true;
		untrack(() => void loadNames());
	});

	async function loadNames() {
		names = await readLibraryNames();
	}

	/**
	 * The line the toolbar prints, or `undefined` on an unscoped view — which is
	 * the whole reason the markup renders nothing there rather than an empty span.
	 */
	const scopeLine = $derived(libraryScopeLine(query?.libraries ?? [], names));

	/**
	 * The services read, and it is allowed to fail. Nothing here is gated on it
	 * except the wording of an empty state, and `browseEmptyState` has an answer
	 * for exactly that case: it says the reason is unknown rather than asserting
	 * this type is empty on the strength of a read that did not answer.
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

<svelte:head>
	<title>{typeLabel === '' ? 'Not a media type' : typeLabel} · UsArr</title>
</svelte:head>

<!--
	NO <h2> RESTATING THE PAGE NAME. The shell's toolbar already renders it and
	the same string is the h1 inside main, so a heading here would put it on
	screen three times. Standing rule on every screen.
-->

{#if route.k === 'unknown-type'}
	<!--
		⚠️ AN HONEST REFUSAL, AND NO REQUEST. `media_type` is a closed enum and an
		unrecognised value is a 400 rather than an unfiltered page, so the answer to
		this address is already known: asking would spend a round trip to be told
		it, and would put the server's wording on a mistake made in the address bar.
		The exits are the six that do exist, as real links.
	-->
	<section class="section">
		<div class="empty">
			<h2 class="empty__title">“{route.value}” is not a media type</h2>
			<p class="empty__text">
				UsArr files everything it replicates under six media types, and this address names something
				else. Nothing was asked of the server: the six below are the whole list.
			</p>
			<div class="empty__actions">
				{#each MEDIA_TYPES as type (type)}
					<a class="btn" href={resolve('/library/[type]', { type })}>{mediaTypeLabel(type)}</a>
				{/each}
			</div>
		</div>
	</section>
{:else if route.k === 'too-many-libraries'}
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
			Everything UsArr has replicated as {typeLabel.toLowerCase()}, from every service that
			catalogues it. It renders from SQLite, so it never waits on a service.
		</p>
		{#if feed !== undefined && feed.loaded && feed.items.length > 0}
			<!--
				`so far` is the honest suffix while a cursor is outstanding: the endpoint
				is keyset-paginated and never sends a total, so the only number this
				screen has is how many rows it holds. Printing it bare would claim this
				type is that size.
			-->
			<p class="pagehead__meta">
				{feed.items.length}
				{feed.items.length === 1 ? 'item' : 'items'}{more ? ' so far' : ''}
			</p>
		{/if}
	</div>

	<div class="toolbar">
		<label class="toolbar__label" for="grid-sort">Sort</label>
		<!--
			⚠️ THE OPTIONS ARE WHAT THE SERVER CAN ACTUALLY SERVE FOR THIS TYPE, AND
			A TO Z IS MISSING FOR MUSIC. `sort_title` walks `(kind, sort_title, id)`
			and SQLite cannot supply ORDER BY from an index whose leading column is
			constrained by `IN`, so the order needs a media type of exactly ONE
			`work.kind` — and Music is two, artists AND albums. Offering the option
			anyway would put a 400 behind a control that looks like the others.
			`browseSortsFor` derives this from the kind count, so it is the store's
			own `len(kinds) != 1` rather than a hard-coded exception for one word.
		-->
		<select id="grid-sort" class="select" value={query.sort} onchange={onSort}>
			{#each sorts as sort (sort)}
				<option value={sort}>{browseSortLabel(sort)}</option>
			{/each}
		</select>
		{#if sortNote}
			<!--
				⚠️ STATED, NOT DROPPED SILENTLY. Music shipped with two options and no
				reason while every other type here offered three, which reads as a screen
				missing a control rather than an order the store cannot serve. The option
				stays out of the select above — the refusal is knowable before a request
				is sent, so offering it and answering with a banner would spend a round
				trip — and this is the sentence that says so. It is `$lib/librarygrid`'s
				so a test can read it, it is NOT the all-types screen's sentence (that one
				claims to span every media type and this view spans one), and it is not
				the server's 400 text, which names a wire parameter and `year` to a reader
				who asked about neither.
			-->
			<p class="toolbar__note toolbar__label">{sortNote}</p>
		{/if}

		{#if query.libraries.length > 0}
			<span class="toolbar__spacer"></span>
			<!--
				THE SCOPE NAMES THE LIBRARY, NOT ITS SLUG. ARCHITECTURE §17.8 never
				renders a slug anywhere on the Libraries screen — *"The row's identifier
				is not rendered as a path … Drop the slash and the mono face"* — so a
				user arriving here on a `?lib=` link would otherwise be scoped by a
				string the UI has never shown them. The name comes from
				`GET /api/v1/libraries`, which publishes `name` and `slug` together
				(`reference/http-api.md` §2.2), and the slug is the fallback whenever it
				has not answered: a scoped view that cannot name its scope still says it
				is scoped.

				⚠️ THIS IS THE VIEW DESCRIBING ITSELF, NOT §8.1's ScopeChip. It states one
				scope and clears it; it does not select, does not multi-select and claims
				nothing about scope anywhere else. The chip stays unbuilt on a wire
				precondition (DESIGN-DIRECTION §9.7), and the address is honoured whether
				or not the control that writes it exists.
			-->
			<span class="toolbar__label">{scopeLine}</span>
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry a query string; page.url.pathname is already resolved -->
			<a class="btn btn--sm" href={clearScopeHref}>Show every library</a>
		{/if}
	</div>

	<section class="section">
		{#if error}
			<div class="banner banner--err" role="alert">
				<Icon name="x-circle" />
				<div class="banner__body">
					<div class="banner__title">This list could not be read</div>
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
				`two-line` below 760 px, which §9.1 gives to a list that is SCANNED
				rather than read one record at a time.

				`total` IS PASSED ONLY ON THE LAST PAGE, and the omission is the honest
				answer rather than a gap. ARIA defines `aria-rowcount="-1"` for a total
				that is genuinely unknown, and it is unknown here by construction: a
				keyset endpoint never says how many rows exist, and this one has no facet
				count either (http-api.md §7.1).
			-->
			<List
				label={`${typeLabel} in your library`}
				columns={COLUMNS}
				rows={feed.items}
				key={(item: RecentItem) => String(item.id)}
				total={more ? undefined : feed.items.length}
				rowIntrinsic={ROW_INTRINSIC_GRID}
				stack="two-line"
				state={feed.items.length === 0 ? 'empty' : 'default'}
				emptyTitle={empty.title}
				emptyText={empty.text}
				emptyActions={emptyExits}
				hasMore={more}
				loadingMore={loading}
				onloadmore={loadPage}
				cell={gridCell}
			/>
		{/if}
	</section>
{/if}

{#snippet emptyExits()}
	<!--
		§17.3 keeps the FIX on the screen that owns it: what is connected decides
		whether this type can ever have rows, and Services owns that. A scoped view
		gets its scope offered back as well, because "no comics in these libraries"
		and "no comics" are different facts and only one of them is about the
		catalogue.
	-->
	<a class="btn btn--primary" href={servicesPath}>Open Services</a>
	{#if query !== undefined && query.libraries.length > 0}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a ResolvedPathname cannot carry a query string; page.url.pathname is already resolved -->
		<a class="btn" href={clearScopeHref}>Show every library</a>
	{/if}
{/snippet}

{#snippet gridCell(item: RecentItem, column: ListColumn)}
	{#if column.id === 'title'}
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
			§17.2's Have column, drawn by the component Home's Block C, the Recently
			added screen and the Search results table all draw it from. Every decision
			in it belongs to `$lib/library.haveCell` and to `$lib/HaveCell.svelte`: the
			tick that must never fire on `total: null`, the gap figure that carries
			§9.5's warn role, and the row nothing has counted, which carries no glyph
			at all. An inline copy of that chain here would be the fourth one, and the
			first two had already come apart in their styling before the component
			existed.
		-->
		<HaveCell {item} />
	{:else if column.id === 'added'}
		<!--
			⚠️ AN UNDATED ROW IS REAL AND RENDERS AS UNDATED. Kavita reaches that
			state with one absent `created` field, and under the `added_at` order the
			server sorts such rows LAST rather than first. Absolute and relative
			together, per §17.3: one identifies the moment, the other answers "how long
			ago" without arithmetic.
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
