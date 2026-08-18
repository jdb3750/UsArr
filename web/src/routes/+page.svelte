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
	 *   Block C   recently added        one unified table      DRAWN in `library` mode
	 *
	 * AND TWO THINGS THAT ARE NOT BLOCKS, added because the blocks above leave
	 * this screen with nothing to do on the install the owner actually has:
	 *
	 *   Search      a release-search entry point, drawn when an indexer exists
	 *   Recent grabs  GET /api/v1/grabs/recent, hidden when empty
	 *
	 * ⚠️ RECENT GRABS IS NOT BLOCK C AND MUST NEVER BE LABELLED AS IT. Block C
	 * is `Recently added` — one unified table over the CATALOGUE, sorted by
	 * `added_at DESC`, with a Type column — and a grab is not an item that
	 * arrived. `$lib/requests`' own KNOWLEDGE_STOPS_NOTE is the whole reason:
	 * UsArr stops watching at the moment Prowlarr accepts the release, so it
	 * does not know whether a single byte followed. A list of grabs under the
	 * words "recently added" would assert exactly the thing UsArr has gone to
	 * some trouble not to claim. It used to occupy Block C's slot on an install
	 * with no catalogue; Block C now holds that slot where there is a catalogue
	 * to hold it, and the grabs list sits below under its own heading.
	 *
	 * NEITHER IS A FOURTH BLOCK IN THE SENSE ADR-0028 BOUNDS. The decision fixes
	 * Home's height at O(1) in the number of MEDIA TYPES, which is what six
	 * per-type strips broke; both of these are single regions whose height is
	 * independent of how many types exist, and the search entry point is not a
	 * region at all — it is one control inside the state block that was already
	 * drawn here, replacing the `Search indexers` button that stood in the same
	 * place. Nothing was added beside the three blocks that could not be put
	 * inside something already on the screen.
	 *
	 * WHY BLOCK A IS STILL ABSENT WHILE BLOCK C IS DRAWN, AND WHY THAT IS THE
	 * CORRECT RENDERING RATHER THAN A HALF-FINISHED ONE. The two blocks want
	 * different reads and only one of them exists. `GET /api/v1/library/recent`
	 * is Block C's, and it landed; Block A's per-type rollup is its own read and
	 * has not, so every count in it would have to be invented. DESIGN-DIRECTION
	 * §9.6 closes that off in as many words — never fabricated data in a shipped
	 * product surface — and a zeroed table and a skeleton shimmer are the same
	 * fabrication with different punctuation. §17.2's own hard rule reaches the
	 * same place from the other side: a media type the user does not have is not
	 * shown AT ALL, not in Block A, not in the sidebar, not as a search group,
	 * and nothing on this screen can yet say which types the user has.
	 *
	 * ⚠️ §17.2 DOES specify Block A's sourceless rows for a v0.1 install — the
	 * type, `no catalogue source connected`, the service that will populate it,
	 * the milestone it arrives in, and a link to Add (§17.2) — and that is NOT
	 * what is missing here. ⚠️ THE EXEMPLAR HERE READ `Comics · no catalogue
	 * source · Kavita · after v0.1 · Add`, AND IT IS FALSE ON BOTH OF ITS
	 * CLAIMS: comics HAS a catalogue source, and that source is IN v0.1.
	 * ADR-0041 made Kavita v0.1's one catalogue source and the sync core's first
	 * adapter, so §16 — authoritative for scope — puts v0.1's catalogue at books
	 * and comics/manga (ARCHITECTURE §16.0, "the catalogue is books and
	 * comics/manga, because Kavita is the source that ships"), and §16.1's
	 * post-v0.1 table no longer lists Kavita because it moved INTO v0.1 rather
	 * than being cut. §17.2 has stopped naming which types are sourceless at all
	 * and defers to §16, so the exemplar has to come from there: `Music · no
	 * catalogue source connected · Navidrome · after v0.1 · Add`, Navidrome
	 * being §16.1's slot #1 and the source that lights music up. Those rows are
	 * the shape for an install where the OTHER rows carry real counts, and no
	 * per-type count is readable yet — not for books and comics either, whose
	 * counts a rollup read would now have something true to put in.
	 * Rendering six rows of which six are `no catalogue source` is not §17.2's
	 * screen; it is a table with no data in it, and rule 13's own bound — the
	 * ban is on a region that says NOTHING — does not rescue a block whose every
	 * row says the same nothing. The block arrives with the read that fills it.
	 * §17.7's `partial` and `stale` states arrive with a per-instance sync clock,
	 * which is a different read again and is not one of the two above.
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
	 *   library          at least one library-bearing service. REACHABLE: a
	 *                    `kavita` instance carries role `library`, which is what
	 *                    Block C is drawn off. See $lib/home, whose own note
	 *                    records the day this stopped being hypothetical.
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
	 * THAT RULE IS ALSO WHY THERE IS NO SERVICES SUMMARY HERE. "Show services
	 * only when something is wrong" is a block this screen already has: Block B
	 * IS that block, it is hidden when empty by ADR-0028, and its rows already
	 * carry the instance, UsArr's own word for the state, the upstream's
	 * verbatim text and a link to the row that owns the fix. A second region
	 * over the same predicate would be the green "all good" panel arriving by
	 * another door. Nothing was added for it.
	 *
	 * AND THE SAME RULE BOUNDS WHAT A RECENT-GRABS ROW MAY OFFER HERE: nothing.
	 * `Search again` and `Open Services` are actions §17.5 puts on the Requests
	 * block, which is the canonical record; Home shows the ten most recent as a
	 * summary and links to it. That is a deliberate narrowing rather than an
	 * omission — see the columns below, and the omitted `actions` prop on
	 * `$lib/RecentGrabs.svelte`, which is the whole of how it is expressed.
	 *
	 * THE CSP FORBIDS INLINE STYLE ATTRIBUTES, so nothing here writes one. The
	 * list primitive sets its custom properties through the CSSOM; everything
	 * else on this screen is a class.
	 */
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, fetchRecentGrabs, fetchServicesHealth, type RecentGrab } from '$lib/api';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import RecentGrabs from '$lib/RecentGrabs.svelte';
	import { LOAD_MORE_PAGE_SIZE, NOTHING, type ListColumn } from '$lib/list';
	import {
		appendPage,
		cursorRejected,
		EMPTY_RECENT_FEED,
		fetchRecentPage,
		hasMore,
		haveCell,
		mediaTypeLabel,
		nextRequest,
		type RecentFeed,
		type RecentItem
	} from '$lib/library';
	import {
		attention,
		headline,
		homeMode,
		HOME_SEARCH_SCOPE_NOTE,
		type AttentionRow,
		type HomeMode
	} from '$lib/home';
	import { LIVE_SEARCH_LIMIT, LiveSearch, type LiveRegion } from '$lib/livesearch';
	import { formatWhen, KNOWLEDGE_STOPS_NOTE, requestsSearchHref } from '$lib/requests';
	import { atCapacity, fetchSearch } from '$lib/search';
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

	/**
	 * RECENT GRABS ON HOME IS THREE COLUMNS, NOT THE REQUESTS BLOCK'S SIX, and
	 * the three that are dropped were dropped for a stated reason each rather
	 * than to make the table fit.
	 *
	 *   Indexer   identifies WHICH tracker answered, which is what you need when
	 *             you are deciding whether to trust or re-run a result. Home is
	 *             not where that decision gets made.
	 *   Protocol  usenet or torrent, read at the same moment and for the same
	 *             purpose as Indexer.
	 *   Size      the one figure on the row that is about the download rather
	 *             than about the grab, and the column §9.1 gives a reserved
	 *             `.unit` box and a 3ch unit slot to. Carrying that machinery for
	 *             a summary would be the copy this file is trying not to make.
	 *
	 * What survives is the three that answer Home's question — "what did I send,
	 * and did it get sent" — which is `when`, `release` and `outcome`. §17.4's
	 * slot-order invariant holds across the two: identity first, then the
	 * verdict, in the same places.
	 *
	 * ⚠️ EVERY TRACK IS FIXED OR `fr`, WHICH THE DEV GUARD IN `gridTemplate()`
	 * ENFORCES. ADR-0029 makes every row its own grid, so a content-sized track
	 * resolves against its own row's contents and the columns cannot line up —
	 * the failure Block B's `Action` track documents in detail above. `132px` is
	 * the Requests block's own Time width, and it is the same content: an
	 * absolute time over a relative one.
	 */
	const GRAB_COLUMNS: ListColumn[] = [
		{ id: 'when', header: 'Time', width: '132px' },
		{ id: 'release', header: 'Release', width: 'minmax(0, 3fr)' },
		{ id: 'outcome', header: 'Outcome', width: 'minmax(0, 1.9fr)' }
	];

	/** §17.5's own limit for the block, and the endpoint's default. */
	const RECENT_LIMIT = 10;

	/**
	 * BLOCK C's COLUMNS. ONE UNIFIED TABLE ACROSS EVERY MEDIA TYPE (ADR-0028),
	 * which is why `Type` is a COLUMN here rather than a heading over a region:
	 * a sixth media type adds rows to this list, not a sixth thing to scan.
	 *
	 * ⚠️ `Type` RENDERS FROM `media_type` AND NEVER FROM `kind`. §17.2 states
	 * that the Tier 1 client index carries no format, so a browser cannot split
	 * ebooks from audiobooks: both are `kind: 'book'` and only `edition.format`
	 * separates them, which is not on this wire. `$lib/library` holds the rule
	 * and the vocabulary so a test can read them.
	 *
	 * ⚠️ EVERY TRACK IS `fr` OR A FIXED RESERVE, WHICH THE DEV GUARD IN
	 * `gridTemplate()` ENFORCES. ADR-0029 makes every row its own grid, so a
	 * content-sized track resolves against its own row's contents and the header
	 * cannot agree with the body. Block B's `Action` track above documents the
	 * measured failure in detail.
	 *
	 * `132px` on `Added` is NOT a new measurement. It is the Time width the
	 * recent-grabs table below already carries, for the same content in the same
	 * shape: an absolute time with a relative one under it. A new fixed reserve
	 * would need its own measurement; reusing a measured one for identical
	 * content does not.
	 *
	 * `stackLine` is §9.1's TWO-LINE phone fork, which it gives to a list that is
	 * SCANNED rather than read one record at a time. §17.2 asks for "the same
	 * small-multiple row as search", and this is that row: the title identifies
	 * the line, and the type and the date are the two secondary fields worth the
	 * second line. `Type` drops its stacked label because the value is the row's
	 * own identity and the word `Type` only restates it.
	 */
	const RECENT_COLUMNS: ListColumn[] = [
		{
			id: 'type',
			header: 'Type',
			width: 'minmax(0, 0.9fr)',
			stackLabel: false,
			stackLine: 2
		},
		{ id: 'title', header: 'Title', width: 'minmax(0, 3.2fr)', stackLabel: false, stackLine: 1 },
		{ id: 'year', header: 'Year', width: 'minmax(0, 0.6fr)', align: 'end', stackLine: 'hidden' },
		{ id: 'have', header: 'Have', width: 'minmax(0, 1.7fr)', stackLine: 'hidden' },
		{ id: 'added', header: 'Added', width: '132px', stackLine: 2 }
	];

	/**
	 * A Block C row carries a sub-line wherever the data has one (the relative
	 * time under the absolute one, a gap figure under a fraction), so it is the
	 * same two-line shape as Block B's rows above and takes the same measured
	 * figure. `ROW_INTRINSIC`'s default is measured on a ONE-line row and would
	 * be wrong by half, which shows as scroll-height jitter.
	 */
	const ROW_INTRINSIC_RECENT = 44;

	const servicesPath = resolve('/services');
	const requestsPath = resolve('/requests');
	const searchPath = resolve('/search');

	let mode = $state<HomeMode | undefined>(undefined);
	let services = $state(0);
	let rows = $state<AttentionRow[]>([]);
	let loadError = $state('');

	/**
	 * WHAT THE USER TYPES. It does two things now, and it did one before.
	 *
	 * ⚠️ THIS BLOCK SAID *"IT NEVER LEAVES THIS SCREEN AS A QUERY. Home does not
	 * search: it navigates"*, AND THAT IS NO LONGER TRUE. The owner asked for
	 * the other half: *"i feel like the search should be realtime on the
	 * homepage. like filter real time type shit"*, with Navidrome's instant
	 * filtering as the reference. So every keystroke now goes to `live.type()`
	 * as well, which filters your own library into the region below the box.
	 *
	 * THE NAVIGATION IS UNCHANGED AND IS STILL THE POINT. Enter and the button
	 * still hand the string to `routes/search` through `?q=`, which is where the
	 * complete result list lives: §6.5 gives that endpoint no second page, the
	 * Search screen asks for the documented maximum of 100, and this region asks
	 * for ten. Home did not become the Search screen; it got the first ten rows
	 * of it without a navigation.
	 */
	let query = $state('');

	/**
	 * THE LIVE REGION'S STATE, AND EVERY DECISION BEHIND IT IS IN `$lib/livesearch`.
	 *
	 * What is here is two `$state` slots and a callback that fills them. What is
	 * NOT here, deliberately, is the debounce, the sequence guard, the abort, the
	 * floor below which nothing is sent and the delay before a screen reader is
	 * told anything: `vitest.config.ts` is `environment: 'node'` with no Svelte
	 * plugin, so a race guard living in this file would be a race guard no test
	 * could resolve two answers out of order against. `livesearch.test.ts` does
	 * exactly that, and fires with the guard removed.
	 */
	let liveRegion = $state<LiveRegion>({ k: 'dormant' });
	let liveAnnouncement = $state('');

	const live = new LiveSearch({
		search: (text, signal) => fetchSearch(text, LIVE_SEARCH_LIMIT, signal),
		onchange: (region, announcement) => {
			liveRegion = region;
			liveAnnouncement = announcement;
		}
	});

	/**
	 * Recent grabs. `grabsLoaded` is separate from `grabs.length` on purpose —
	 * an empty list and an unread list mean opposite things, and the Requests
	 * block draws the same distinction for the same reason. Before the read
	 * lands, neither the section nor an empty state is drawn.
	 */
	let grabs = $state<RecentGrab[]>([]);
	let grabsLoaded = $state(false);
	let grabsError = $state('');

	/**
	 * WHETHER THE RECENT-GRABS REGION IS ON SCREEN AT ALL, DERIVED ONCE.
	 *
	 * The separator below has to draw only where it has two regions to sit
	 * between, so it needs this predicate — and a second hand-written copy of
	 * the `{#if}` conditions is exactly the pair of facts that must agree and
	 * therefore can disagree, which is the argument `grabsLoaded` above is
	 * already written against. So the arms read these and nothing restates them:
	 * `grabsListed` is the list arm, `grabsDrawn` is either arm.
	 */
	const grabsListed = $derived(grabsLoaded && grabs.length > 0);
	const grabsDrawn = $derived(grabsError !== '' || grabsListed);

	/**
	 * BLOCK C's STATE. `$lib/library`'s `RecentFeed` is the whole of the paging
	 * position: the rows read so far, the cursor for the next page, and whether
	 * the server has answered at all. There is deliberately no `hasMore` boolean
	 * beside it, because "is there more" is `cursor !== undefined` and nothing
	 * else, and two fields that must agree are two fields that can disagree.
	 */
	let recent = $state<RecentFeed>(EMPTY_RECENT_FEED);
	let recentLoading = $state(false);
	let recentError = $state('');
	/** The server's own `action`: the one thing it says fixes this. */
	let recentAction = $state('');
	/** Whether the failure was the server rejecting a cursor UsArr sent it. That
	 * one is not retryable and gets a restart control rather than silence. */
	let recentRejected = $state(false);

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

	/**
	 * The href the submit button would follow with scripting off, and the one
	 * `submit()` navigates to with it on. Built from `resolve('/search')` plus
	 * the one parameter that makes the link work — §17.4's mechanism, through
	 * `$lib/requests`' `requestsSearchHref` rather than a second copy of it.
	 *
	 * ⚠️ THE HELPER'S NAME SAYS `requests` AND THE BASE PASSED TO IT IS `/search`,
	 * WHICH IS DELIBERATE AND IS NOT A MISTAKE TO TIDY. What it builds is the
	 * `?q=` contract — trim, encode, and a BARE path for an empty query so the
	 * destination shows its idle state instead of echoing a search for nothing —
	 * and that contract is one contract with two readers: `routes/requests` and
	 * `routes/search` both parse `?q=` in `onMount` the same way. Its name
	 * records where it was first needed, not where it may be used, and renaming
	 * it would touch three call sites and a test file to say nothing new.
	 */
	const searchHref = $derived(requestsSearchHref(searchPath, query));

	const recentMore = $derived(hasMore(recent));

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
		// BLOCK C IS ASKED FOR ONLY WHERE THERE IS SOMETHING TO ASK ABOUT, which
		// is principle 3 rather than an optimisation: with no library-bearing
		// service there is no catalogue, the `search-and-grab` block below already
		// says so in words, and a request whose only possible answer is an empty
		// list is a request that teaches the screen nothing.
		//
		// `!recent.loaded` is what keeps this to ONE read. `load()` is on a
		// 60-second timer because Block B's relative clauses go stale; a
		// recently-added table has no such clause, so re-reading it every minute
		// would be a repeated query for a table that changes only while the user
		// is on another screen. Recent grabs is excluded from the timer for the
		// same reason.
		if (mode === 'library' && !recent.loaded) void loadRecent();
	}

	/**
	 * ONE PAGE OF BLOCK C, AND THE STOP RULE IS NOT HERE.
	 *
	 * `nextRequest` owns it: it answers `undefined` when there is nothing left to
	 * ask for, which happens exactly when the server omitted `next_cursor`. This
	 * function cannot short-page itself into stopping early, because it never
	 * looks at `items.length` at all. The rule and its test live in
	 * `$lib/library`, where a rule can be read rather than inferred from an
	 * `{#if}`.
	 *
	 * A LOCAL SQLITE READ, which is what lets Home make it at all (principle 1).
	 * `internal/httpapi/library.go` says so at its own declaration: one statement
	 * per page, no *Arr, no metadata provider, no image fetch.
	 */
	async function loadRecent() {
		const request = nextRequest(recent, LOAD_MORE_PAGE_SIZE);
		if (request === undefined) return;
		recentLoading = true;
		try {
			recent = appendPage(recent, await fetchRecentPage(request));
			recentError = '';
			recentAction = '';
			recentRejected = false;
		} catch (error) {
			recentError = error instanceof ApiError ? error.detail : String(error);
			recentAction = error instanceof ApiError ? error.action : '';
			// ⚠️ A REJECTED CURSOR IS NOT RETRIED, AND THE SERVER'S OWN COMMENT IS
			// THE REASON: it answers 400 rather than silently resetting to page one,
			// because resetting "turns a stale bookmark into a Load-more loop that
			// re-serves the first page for ever and looks like the list is stuck".
			// Retrying here would build that loop on the other side of the wire. The
			// screen surfaces the action and offers a restart the user presses.
			recentRejected = cursorRejected(error);
		} finally {
			recentLoading = false;
		}
	}

	/** Start again from the newest items, which is the action the server names.
	 * It is a request with NO cursor, so it cannot fail the way the last one
	 * did. */
	function restartRecent() {
		recent = EMPTY_RECENT_FEED;
		recentError = '';
		recentAction = '';
		recentRejected = false;
		void loadRecent();
	}

	/**
	 * A LOCAL SQLITE READ, which is what lets Home make it at all. Principle 1
	 * forbids a render path that blocks on an *Arr; `GET /api/v1/grabs/recent`
	 * reads UsArr's own `provenance` and `audit_log` rows and touches no
	 * upstream, which `$lib/api` says in as many words at its declaration.
	 *
	 * It is deliberately NOT on the 60-second timer that re-reads health. That
	 * timer exists because Block B's relative clauses go stale — "retrying in 4
	 * minutes" is wrong a minute later — and a grab list has no such clause: the
	 * absolute time is fixed and the relative one is recomputed from `now`,
	 * which the timer already moves. Polling it would be a repeated read for a
	 * table that only changes when the user is on another screen.
	 */
	async function loadGrabs() {
		try {
			const recent = await fetchRecentGrabs(RECENT_LIMIT);
			grabs = recent.grabs;
			grabsError = '';
		} catch (error) {
			grabsError = error instanceof ApiError ? error.detail : String(error);
		} finally {
			grabsLoaded = true;
		}
	}

	/**
	 * Enter or the button. `goto` rather than a full navigation, because the
	 * destination is a route in this app and reloading the shell to reach it
	 * would throw away the SPA for no gain.
	 *
	 * ⚠️ THE NATIVE SUBMIT IS PREVENTED ONLY BECAUSE THE `<a>` BESIDE IT IS
	 * REAL. The button's own `formaction` is the same href, so a build with
	 * scripting off still reaches Search with the query on it; this handler
	 * replaces a full page load with a client navigation and nothing else. An
	 * empty query goes to `/search` bare, which lands on that screen's idle
	 * state rather than on a search for nothing.
	 */
	function submit(event: SubmitEvent) {
		event.preventDefault();
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- a resolve()'d path plus ?q=, which a ResolvedPathname cannot carry; see $lib/requests requestsSearchHref
		void goto(searchHref);
	}

	onMount(() => {
		void load();
		void loadGrabs();
		const timer = setInterval(() => {
			now = new Date();
			void load();
		}, 60_000);
		// `live.dispose()` cancels the pending debounce and aborts whatever read
		// is in flight, so a keystroke made on the way out cannot resolve into a
		// component that has gone.
		return () => {
			clearInterval(timer);
			live.dispose();
		};
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
	THE SEARCH ENTRY POINT.

	⚠️ IT IS A ROUTE, NOT A SEARCH FIELD, AND THAT IS THE WHOLE DESIGN.
	DESIGN-DIRECTION §8.3 holds library search and release search apart —
	"Merging them is how a 0 ms local query ends up waiting on a 30 s indexer" —
	so nothing here fans out to an indexer, opens an SSE stream or renders a
	result. Submitting navigates carrying `?q=`, which is the mechanism §17.4
	already fixes for the `Search indexers →` action on a result row.

	⚠️ IT USED TO NAVIGATE TO REQUESTS, AND THE OWNER MOVED IT: *"the search on
	home should be searching your own library. this is actually what i was
	talking about when i said we have a lot of search functions"*. So the
	destination is `/search` and the corpus is the local one. §8.3 is not bent by
	that; it is satisfied more directly than before. The rule §8.3 states is that
	the two searches stay APART, and Home now holds exactly one of them — the 0 ms
	local one — while Requests stays the one release-search surface in UsArr. The
	old wiring kept Home a local read only by deferring the question one
	navigation; this one answers it locally at both ends.

	IT SAYS WHAT IT SEARCHES, in the note under it, and the reason INVERTED with
	the destination rather than going away. ⚠️ THE REASON USED TO READ "an
	unlabelled input on a home screen reads as a search of your own library by
	default — and on this install a search of your own library is the one thing
	that cannot work". Both halves are dead. The second half died at `04a28a4`,
	which gave `/api/v1/search` to the local corpus (`internal/httpapi/
	server.go:300` routes it to `handleLibrarySearch`, against `:291`, which
	routes the fan-out to `handleSearch` on `/api/v1/releases/search`) and at
	`cbf82bc`, which shipped the screen over it. The first half stopped being an
	argument for the note the moment the box became the thing the user was
	assuming it was. WHAT REPLACES IT is the owner's own second sentence: there
	are several searches in this product, and the one defect a user cannot
	recover from is not knowing which one they are typing into. The note names
	the corpus for that reason now, not to correct an assumption.

	DRAWN OFF THE MODE, NOT OFF `hasIndexer`, AND THAT SWAPPED WITH THE
	DESTINATION TOO. ⚠️ IT USED TO BE `hasIndexer` — at least one configured
	service is an indexer — which was the right precondition for a box that
	reached indexers and is the WRONG one for a box that reads a catalogue, in
	both directions: a Prowlarr-only install would draw a library search over a
	library that cannot exist, and a Sonarr-only install would refuse to draw one
	over a catalogue that does. The precondition is unchanged in principle —
	something can answer the query — so it is now `mode === 'library'`, which is
	the same gate Block C uses for the same reason and is $lib/home's own test for
	whether a library-bearing service is configured at all. A box with nothing
	behind it is invented status wearing a control.

	NOT A HERO, and §1.5's table is the checklist it is written against: no
	oversized centred input, no illustration, no "Get started", no stat banner.
	It is an <h2> at --text-lg, a native <input type="search"> and a submit
	button — and, since the owner asked for it, a wordmark left-pinned above the
	pair.
-->
{#if mode === 'library'}
	<section class="section" id="home-search">
		<div class="section__head">
			<h2>Search your library</h2>
		</div>
		<!--
			THE WORDMARK. The owner asked for a Google-shaped Home — the product name
			above the search box, in a serif — and confirmed the serif explicitly.

			⚠️ IT STAYS AT --text-xl (20px) AND IS NOT HERO-SIZED, which the owner
			also accepted. 20px is the largest type anywhere in this application with
			no exemption, and the type scale's seventh step was DELETED to enforce
			that; a wordmark is not the case that reopens ADR-0025. What makes it
			read as a wordmark instead of as another heading is therefore the
			FAMILY, which costs no pixels — not size, which does, and not centring,
			which was ruled against (see the `.wordmark` rule below).
			`--weight-xl` and `--leading-xl` come along unchanged, so this is the
			page-title pairing with one property swapped.

			NO BRAND HUE. It is `--fg`, the same ink as body text, because this
			product has no accent colour and inventing one for its own name is the
			first step of the thing §1.5 sends back to the landing page.

			NO HERO BAND. There is no oversized vertical padding and no tall empty
			region for it to float in: it is one line of text with normal section
			spacing, sitting directly on the row below it.

			IT IS aria-hidden, ON THE PRECEDENT THE TOOLBAR ALREADY SET. `UsArr` is
			already in the accessibility tree three times on this screen — the
			document title, the `topbar__brand` link, and the shell's h1 — and
			+layout.svelte hides `topbar__title` for exactly this reason: the same
			string as furniture and as a heading is a duplicate, not context. A
			screen reader gains nothing from hearing the product name a fourth time
			immediately before the search field, and this is the one element on the
			screen that is pure furniture.
		-->
		<p class="wordmark" aria-hidden="true">UsArr</p>
		<!--
			A REAL FORM WITH A REAL DESTINATION. The button's `formaction` is the
			same href `submit()` navigates to, so the control works without
			scripting and the handler only upgrades a page load to a client
			navigation. `type="search"` is the native control; nothing here is a
			styled <div> pretending to be one.
		-->
		<form class="homesearch" onsubmit={submit}>
			<!--
				A LABEL, NOT A PLACEHOLDER-AS-LABEL: the placeholder is gone the moment
				the field is typed into, and it is the only other string here.

				⚠️ IT DELIBERATELY DOES NOT REPEAT THE HEADING. `Search your library`
				is two elements away in the accessibility tree, and naming the field
				with it too makes a screen reader announce the same words twice in a
				row — heading, then edit field — which is noise rather than context.
				The heading supplies the scope for anyone navigating by heading; the
				label supplies what goes IN the box, which is the thing the heading
				does not say.

				⚠️ AND IT IS THE SEARCH SCREEN'S OWN LABEL AND PLACEHOLDER, WORD FOR
				WORD, which is the point rather than a coincidence. This box submits
				INTO `routes/search`; two forms that are one form across a navigation
				may not describe their input differently, or the user learns that the
				second field wants something else. ⚠️ THEY USED TO READ `Release name`
				and `Release name, or part of one`, which named the corpus this box no
				longer has: a release is a file on an indexer, and nothing in a
				replicated catalogue is one. Requests keeps that vocabulary, because
				Requests keeps the releases.
			-->
			<label class="sr" for="home-query">Search terms</label>
			<!--
				⚠️ `oninput` READS `event.currentTarget.value` AND NOT `query`. Both
				are on this element, and the order Svelte applies the binding in
				relative to a handler on the same event is not something a component
				should depend on; the event's own target is the value that provoked
				the event by definition. `bind:value` stays because `searchHref` and
				the submit path are built from it.

				NOTHING HERE TOUCHES A KEY. There is no `onkeydown`, no
				`preventDefault` and no key filtering on this field, so every
				keystroke, every IME composition and every paste reaches the input
				unmodified and the only thing that happens on top is a read. Escape,
				the type="search" clear control and select-all-delete all arrive as an
				`input` event with an empty value, which is the `dormant` plan.
			-->
			<input
				id="home-query"
				name="q"
				type="search"
				class="searchfield homesearch__input"
				autocomplete="off"
				placeholder="A title, or a name credited on one"
				aria-describedby="home-search-note"
				bind:value={query}
				oninput={(event) => live.type(event.currentTarget.value)}
			/>
			<button type="submit" class="btn btn--primary" formaction={searchHref}>Search</button>
		</form>
		<p class="note" id="home-search-note">{HOME_SEARCH_SCOPE_NOTE}</p>

		<!--
			THE RESULTS, INLINE AND SIMPLY THERE.

			⚠️ NO OVERLAY, NO DROPDOWN, NO ANIMATION. This is not a combobox and it
			is not a floating panel over the page: it is rows in the document flow,
			directly under the field, pushing Block C down as it grows. A floating
			list would need a shadow, a stacking context, an outside-click dismissal
			and a keyboard contract this screen has no use for, and §1.5 rules out
			the shadow on its own. Navidrome is the bar and Navidrome simply shows
			the list.

			⚠️ AND NOTHING IS DRAWN WHILE A READ IS IN FLIGHT. §7.2 Tier 0 is "show
			nothing at all. No skeleton, no spinner, no fade-in" for a read out of
			local SQLite, and between two keystrokes this region is already showing
			a true answer to the previous one. Swapping that for an indicator would
			replace something true with something that only says wait, and drawing a
			`nothing matched` line under a half-typed word would say something false.
			The empty arm below is only ever reached from an answer the server gave
			for the text that is in the box.

			THE ROWS ARE BLOCK C's ROWS, THE SAME COLUMNS AND THE SAME SNIPPET.
			`docs/reference/http-api.md` §6.2 fixes the two wire shapes as identical
			key for key, "deliberately, so one row component renders both Home's
			recently-added table and a search result", and `$lib/search` types
			`SearchItem` as an alias of `RecentItem` rather than re-declaring it. A
			second column set here would be two layouts computed for one row, which
			is the drift ADR-0029 exists to prevent.
		-->
		{#if liveRegion.k !== 'dormant'}
			<div class="liveresults">
				{#if liveRegion.k === 'floor'}
					<!--
						ONE CHARACTER, AND NOTHING WAS ASKED. `internal/store/searchquery.go`
						drops a one-character final token from the keyword leg and keeps
						every token under three characters off the trigram leg, so no leg
						can run and §6.6 says the answer is 200 with no items. This line
						says the input is short; it does not say the library is empty,
						because nothing has been asked that could have found out.
					-->
					<p class="note livenote">One letter is too short to match. Two is enough to start.</p>
				{:else if liveRegion.k === 'error'}
					<!--
						NO `role="alert"`. An assertive role fires on every keystroke while
						a backend is down, which interrupts the person typing over and over.
						The polite region at the foot of this section carries it once, after
						they stop.
					-->
					<div class="banner banner--err">
						<Icon name="x-circle" />
						<div class="banner__body">
							<div class="banner__title">Your library could not be searched</div>
							<div class="banner__text">
								This is a read of UsArr’s own database, so it failing is not a service being slow.
								Your library is not known to be empty; it is unknown.
							</div>
							<p class="verbatim">{liveRegion.message}</p>
							{#if liveRegion.action}<p class="banner__text">{liveRegion.action}</p>{/if}
						</div>
					</div>
				{:else if liveRegion.k === 'nothing'}
					<!--
						THE SERVER ANSWERED AND HAD NOTHING, for the text in the box. One
						line, no box and no illustration: it is replaced by the next
						keystroke, and a block that big flashing on and off under a typing
						hand is the noise this whole region is written against. The full
						zero-results state, with the note about what the matching does and
						the exit to the indexers, is on the Search screen, where the query
						is finished.
					-->
					<p class="note livenote">Nothing in your library matches “{liveRegion.query}”.</p>
				{:else}
					<List
						label="Library search results"
						columns={RECENT_COLUMNS}
						rows={liveRegion.results.items}
						key={(item: RecentItem) => String(item.id)}
						total={liveRegion.results.items.length}
						rowIntrinsic={ROW_INTRINSIC_RECENT}
						stack="two-line"
						cell={recentCell}
					/>
					{#if atCapacity(liveRegion.results)}
						<!--
							§17.4 rule 3's finding from Baymard: silent truncation is what
							makes a user believe they have seen everything. The number is the
							ECHOED limit and never the one that was sent, because §6.3 clamps
							silently and only the echoed value describes the answer in hand.
							It claims no total, because nothing on the wire carries one.
						-->
						<p class="note livenote">
							The first {liveRegion.results.limit} matches are here. Press Search for the rest.
						</p>
					{/if}
				{/if}
			</div>
		{/if}

		<!--
			WHAT A SCREEN READER IS TOLD, AND IT IS TOLD IT LATE AND POLITELY.

			`polite` queues behind whatever is being read rather than cutting into
			it, which is the only acceptable setting for a region that changes under
			a typing hand. `aria-atomic` makes the sentence read as a sentence
			instead of as a changed word.

			⚠️ THE COUNT IS NOT PUBLISHED ON EVERY KEYSTROKE, and politeness alone
			would not prevent that: it decides when each update is spoken, not
			whether it is queued, so a per-keystroke region reads out every
			intermediate count once the user stops. `$lib/livesearch` holds the text
			back until the user has been still for `ANNOUNCE_SETTLE_MS` and withdraws
			it the moment they type again, so one pause produces one sentence.

			IT IS NEVER FOCUSED AND NOTHING IN THIS REGION EVER TAKES FOCUS. No
			element here calls `focus()`, the rows come after the field in document
			order so Tab reaches them only when the user asks, and the region is a
			`<p>` rather than anything focusable.
		-->
		<p class="sr" aria-live="polite" aria-atomic="true">{liveAnnouncement}</p>
	</section>
{/if}

<!--
	BLOCK C. ONE UNIFIED RECENTLY-ADDED TABLE ACROSS ALL TYPES (§17.2 as amended
	by ADR-0028), read from GET /api/v1/library/recent.

	⚠️ ONE TABLE WITH A TYPE COLUMN, NEVER ONE STRIP PER MEDIA TYPE. ADR-0028
	settled that on this project's own arithmetic rather than on the carousel
	research: six strips put about sixteen items above a 900 px fold against the
	design's own twenty-five-item floor, on the screen whose entire job is
	inventory. A sixth media type has to add ROWS here, not a sixth region to
	scan, which is what keeps Home's height O(1) in the number of types.

	DRAWN ONLY IN `library` MODE, and that is principle 3 rather than an
	omission. With no library-bearing service there is no catalogue to have
	recently added anything to, and the Search-and-Grab block below already says
	that in words with the reason attached. Asking the endpoint anyway would put
	an empty table under a heading that promises rows.

	NOTHING IS DRAWN BEFORE THE READ LANDS. No skeleton, no shimmer, no zeroed
	table: §9.6 bans fabricated data in a shipped product surface, and a
	placeholder row is fabricated data with rounded corners. The section appears
	when there is something true to put in it, which on a Tier 0 local read is
	about as long as a frame.

	THE ERROR ARM DRAWS WHERE AN EMPTY ONE WOULD NOT. This is a local SQLite
	read, so it failing says something about UsArr rather than about the library,
	and silence would let the screen imply nothing has ever been catalogued.
-->
{#if mode === 'library'}
	<section class="section" id="home-recent">
		<div class="section__head">
			<h2>Recently added</h2>
			{#if recent.loaded && recent.items.length > 0}
				<!--
					`so far` is the honest suffix while a cursor is outstanding: the
					endpoint is keyset-paginated and never sends a total, so the only
					number this screen has is how many rows it holds. Printing it bare
					would claim the library is that size.
				-->
				<span class="section__count num">
					{recent.items.length}
					{recent.items.length === 1 ? 'item' : 'items'}{recentMore ? ' so far' : ''}
				</span>
			{/if}
		</div>

		{#if recentError}
			<div class="banner banner--err" role="alert">
				<Icon name="x-circle" />
				<div class="banner__body">
					<div class="banner__title">Your recently added items could not be read</div>
					<div class="banner__text">
						This is a local read from UsArr’s own database, so it failing is not an upstream
						problem. Nothing is missing here because an item did not arrive: the list simply could
						not be loaded.
					</div>
					<p class="verbatim">{recentError}</p>
					{#if recentRejected}
						<!--
							⚠️ THE SERVER'S OWN `action`, AND NO AUTOMATIC RETRY. A cursor
							this endpoint did not issue is a 400, deliberately, because a
							silent reset to page one turns a stale bookmark into a Load-more
							loop that re-serves the first page for ever. Sending the same
							cursor again would produce the same answer, so the fix is a
							restart the user presses, and it goes out with no cursor on it.
						-->
						{#if recentAction}<p class="banner__text">{recentAction}</p>{/if}
						<div class="empty__actions">
							<button type="button" class="btn btn--sm" onclick={restartRecent}>
								Start again from the newest items
							</button>
						</div>
					{/if}
				</div>
			</div>
		{/if}

		{#if recent.loaded}
			<!--
				`two-line` below 760 px, which §9.1 gives to a list that is SCANNED
				rather than read one record at a time, and §17.2 asks Block C for "the
				same small-multiple row as search".

				`total` IS PASSED ONLY ON THE LAST PAGE, and the omission is the honest
				answer rather than a gap. ARIA defines `aria-rowcount="-1"` for a total
				that is genuinely unknown, and it is unknown here by construction: a
				keyset endpoint never says how many rows exist. Passing `items.length`
				while a cursor is outstanding is what makes a screen reader say
				"row 3 of 200" when the truth is "row 3 of 4,000".
			-->
			<List
				label="Recently added"
				columns={RECENT_COLUMNS}
				rows={recent.items}
				key={(item: RecentItem) => String(item.id)}
				total={recentMore ? undefined : recent.items.length}
				rowIntrinsic={ROW_INTRINSIC_RECENT}
				stack="two-line"
				state={recent.items.length === 0 ? 'empty' : 'default'}
				emptyTitle="Nothing catalogued yet"
				emptyText="A library-bearing service is connected and UsArr has not written any catalogue rows from it yet. The first full import starts on its own the first time UsArr builds a connection to a service that has never completed one, and on a large library it runs for minutes rather than seconds. This table fills in as those rows are written."
				hasMore={recentMore}
				loadingMore={recentLoading}
				onloadmore={loadRecent}
				cell={recentCell}
			/>
		{/if}
	</section>
{/if}

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
		{@const label = mediaTypeLabel(item.mediaType)}
		<span class="trunc">{label}</span>
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
			zero is a claim about a release date. `NOTHING.empty` is §9.1's word for
			a value that is genuinely empty and unremarkably so.
		-->
		{#if item.year !== undefined}
			<span class="num">{item.year}</span>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{:else if column.id === 'have'}
		<!--
			§17.2's `have / total · N missing`, rendered through §6.3's rule and
			schema.md's polymorphic blob. `$lib/library.haveCell` owns every decision
			in here; this renders what it is handed.

			⚠️ THE TICK IS THE ONE THING THIS CELL MAY NOT GET WRONG. schema.md is
			explicit that `total: null` is not `total: 0` and that §6.3's tick "must
			never fire" on the first, so the mark arrives as a discriminated union
			with a fourth state for a count with no honest denominator, and this
			markup cannot reconstruct a comparison of its own.

			CHROMA MARKS WHAT IS WRONG, NOT WHAT IS FINE (§9.5), which is why the
			complete tick is muted and the gap figure is the one thing here carrying
			the warn role. Every glyph has a word beside it in the accessibility
			tree, because §11 forbids a status glyph with an empty accessible name.

			⚠️ A ROW NOTHING HAS COUNTED CARRIES NO GLYPH AT ALL, AND THAT IS THE
			RULE RATHER THAN A LAYOUT CHOICE. `http-api.md` §1.4.1: an absent
			`availability` means no count has ever been computed, so a consumer
			"must not render an absent blob as `0`, as "none", or as any glyph, bar
			or accessible name that asserts emptiness". The cross belongs to a
			PRESENT blob carrying `have: 0`, which is a measured nothing; this one is
			words, because there is no glyph for "not yet asked".
		-->
		{@const have = haveCell(item)}
		{#each have.lines as line (line.key)}
			<div class="availline">
				{#if line.label}
					<span class="availlabel trunc" title={line.label}>{line.label}</span>
				{/if}
				{#if line.mark.k === 'complete'}
					<span class="muted"><Icon name="check" size="sm" /><span class="sr">complete</span></span>
				{:else if line.mark.k === 'none'}
					<span class="muted"
						><Icon name="x-circle" size="sm" /><span class="sr">none held</span></span
					>
				{:else if line.mark.k === 'fraction'}
					<span class="num">{line.mark.have} / {line.mark.total}</span>
				{:else if line.mark.k === 'partial'}
					<span class="num">{line.mark.have}</span>
				{:else}
					<span class="muted">Not counted yet</span>
				{/if}
			</div>
		{/each}
		{#if have.more > 0}
			<!-- §9.1 caps a cell that renders one line per related object at three
			     plus a count of the rest. A dual-Radarr work has two tiers; a
			     well-catalogued album can have many editions. -->
			<div class="cell-sub">+{have.more} more</div>
		{/if}
		{#if have.missing}
			<div class="cell-sub availgap">{have.missing}</div>
		{/if}
		{#if have.gaps}
			<!-- The contiguity list, which is the number that is always honest: it
			     is computed locally from the issue numbers rather than taken from an
			     upstream's declared total. -->
			<div class="cell-sub trunc" title={have.gaps}>Gaps at {have.gaps}</div>
		{/if}
	{:else if column.id === 'added'}
		<!--
			⚠️ AN UNDATED ROW IS REAL AND RENDERS AS UNDATED. Kavita reaches that
			state with one absent `created` field, and the server sorts such rows
			LAST rather than first so that a missing date cannot claim the top of a
			block whose question is "what did I just get". Absolute and relative
			together, per §17.3: one identifies the moment, the other answers "how
			long ago" without arithmetic.
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

<!--
	THE SEPARATOR BETWEEN `Recently added` AND `Recent grabs`, AND IT IS THERE
	BECAUSE THE DEFECT IS COMPOSITIONAL RATHER THAN LEXICAL. Design ruled on
	Home's composition: both regions stay, `Recently added` keeps its name
	because it is conventional and accurate, and the headings are NOT asked to
	carry the distinction between the two. The screen already tells the truth —
	the columns diverge and so does the typography — and what was missing is
	that two adjacent regions, each opening on a past-tense adverb of recency
	with no copy in between, scan as one thing in two parts. A rule between them
	is the smallest thing that stops that read, and it costs one line.

	IT DRAWS ONLY WHERE IT HAS TWO REGIONS TO SIT BETWEEN. `Recently added` is
	`library` mode only and recent grabs is hidden when empty, so on the install
	that has one and not the other a separator would be a stray rule under
	whatever happened to precede it — a mark whose only content is that it is a
	mark, which is rule 13's own ban read onto a one-pixel region.

	AN <hr> RATHER THAN A BORDER ON THE SECTION BELOW, and the reason is the
	line above rather than semantics: a `border-top` belongs to `#home-grabs`
	and would therefore draw whenever that section does, including in
	`search-and-grab` mode where the thing above it is the search block. The
	separator is a fact about the PAIR, so it is its own element with the pair's
	own condition on it, and `<hr>` is what a thematic break between two content
	regions already means.
-->
{#if mode === 'library' && grabsDrawn}
	<hr class="home-sectionsep" />
{/if}

<!--
	RECENT GRABS, AND IT IS NOT BLOCK C. Block C is `Recently added` over the
	catalogue and is drawn above; this is the local record a grab leaves, read
	from GET /api/v1/grabs/recent. It used to occupy Block C's slot on an install
	with no catalogue, and it has vacated it rather than been deleted: a grab is
	a release handed to a download client, which is a different fact from an item
	that arrived, and it stays worth its own region under its own heading.

	HIDDEN WHEN EMPTY, which is Block B's discipline applied to the one other
	region on this screen that can be empty for an honest reason. A user who has
	connected Prowlarr and grabbed nothing yet is told what to do by the search
	box above; a "No grabs recorded yet" table under it would be a region whose
	only content is the observation that it has none. Komga's `v-if="count > 0"`
	and Navidrome's `LibrarySelector` returning null are the same rule, and §17.2
	adopts both.

	AN UNREADABLE LIST IS NOT AN EMPTY ONE, so the error arm draws where the
	empty one does not. This is a local SQLite read: it failing says something
	about UsArr rather than about your grabs, and silence there would let the
	screen imply nothing has ever been grabbed.
-->
{#if grabsError}
	<section class="section" id="home-grabs">
		<div class="section__head"><h2>Recent grabs</h2></div>
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">Your recent grabs could not be read</div>
				<div class="banner__text">
					This is a local read from UsArr’s own database, so it failing is not an upstream problem.
					Nothing is missing here because a grab did not happen — the list simply could not be
					loaded.
				</div>
				<p class="verbatim">{grabsError}</p>
			</div>
		</div>
	</section>
{:else if grabsListed}
	<section class="section" id="home-grabs">
		<div class="section__head">
			<h2>Recent grabs</h2>
			<span class="section__count num">
				{#if grabs.length < RECENT_LIMIT}
					{grabs.length}
					{grabs.length === 1 ? 'grab' : 'grabs'}
				{:else}
					the {RECENT_LIMIT} most recent
				{/if}
			</span>
			<!--
				§17.4's show-all template: the link names its type and its parent
				scope, so it still says what it does when a screen reader reads it out
				of context in a links list. It is the route to the canonical block —
				the one that carries the size, the indexer, the protocol and the two
				actions a row may offer.
			-->
			<div class="section__actions">
				<a class="btn btn--sm" href={requestsPath}>All recent grabs on Requests</a>
			</div>
		</div>

		<!--
			WHERE UsArr'S KNOWLEDGE STOPS, and it is above the table because it
			governs how every row below is read rather than footnoting them. It is
			$lib/requests' own constant, so Home and Requests cannot drift into
			describing the same rows differently.

			⚠️ NOT_SENT_NOTE IS DELIBERATELY NOT CARRIED HERE. It exists to stop the
			Requests block reading as a complete register of every way a grab can go
			wrong. This block states its own bound in the count beside the heading —
			`the 10 most recent` — and links to the block that carries the sentence,
			so the inference it guards against is already blocked. Repeating a
			three-clause paragraph about audit-log routing on a summary would cost
			more rows than the summary has.
		-->
		<p class="note home-grabnote">{KNOWLEDGE_STOPS_NOTE}</p>

		<!--
			THE SAME COMPONENT THE REQUESTS BLOCK DRAWS, under a narrower projection.
			`$lib/RecentGrabs.svelte` owns the row markup — the three states, the
			conditional sub-lines, the labelled stacking below 760 px — so the two
			screens cannot end up wording the same row differently, which is the one
			thing that would make this summary worse than no summary.

			NO `actions` PROP, AND ITS ABSENCE IS THE DECISION. §17.3 states a problem
			canonically once per screen: `Search again` starts a fresh fan-out only
			Requests can run, and `Open Services` is Block B's job above when the
			fault is UsArr's own configuration. `outcome.nonAction` still renders,
			because naming the non-action is a statement about the row rather than a
			control, and dropping it would leave a bare error chip.
		-->
		<RecentGrabs
			{grabs}
			columns={GRAB_COLUMNS}
			{now}
			total={grabs.length < RECENT_LIMIT ? grabs.length : undefined}
		/>
	</section>
{/if}

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
				Prowlarr AND Kavita are named, and naming exactly those two is a
				correctness call rather than brevity.
				`internal/httpapi.serviceKinds` (internal/httpapi/services.go:50-53)
				accepts exactly two kinds — `prowlarr` with role `indexer`, `kavita`
				with role `library` — so a sentence offering Sonarr, Radarr or a
				media server BY NAME here would send a brand-new user to a dialog
				that refuses all three.

				⚠️ KAVITA WAS DELIBERATELY LEFT OUT OF THIS SENTENCE, AND THE
				CONDITION FOR PUTTING IT IN WAS WRITTEN HERE RATHER THAN ON A
				CALENDAR. The text that stood here read: "Kavita is not named
				either, and that is the same call applied honestly: a Kavita can be
				ADDED today and nothing imports from it yet, so pointing the very
				first thing a new user sees at a catalogue that stays empty would
				promise what the milestone does not yet ship. So the condition for
				naming it is in the sentence rather than on a calendar: Kavita
				belongs here the moment adding one gets the user a catalogue, which
				is the same test that keeps it out today." THAT TEST IS NOW MET, so
				the condition was SATISFIED rather than dropped: `internal/libsync`
				imports a Kavita catalogue, and `cmd/usarr` fires that import when a
				Kavita client stack is built (`bootstrapImport`, wired at
				cmd/usarr/services.go, gated on `last_full_sync_at`). Adding a
				Kavita gets the user a catalogue, which is the sentence's own test.

				⚠️ AND WHAT IS STILL NOT CLAIMED, because the corrected sentence is
				one word away from claiming it. One adapter, one trigger a user can
				reach, and NO TIMER — `cmd/usarr/import.go` says exactly that. The
				import runs at most once per instance per database, there is no
				scheduler and no periodic re-import, and the manual trigger is a Go
				call with no HTTP or CLI route in front of it. "once, when you
				connect it" is the whole of what may be said here, and it must not
				grow into a promise of freshness.
			-->
			<p class="empty__text">
				UsArr talks to the services you already run, and none of them is connected yet.
			</p>
			<div class="empty__actions">
				<a class="btn btn--primary" href={servicesPath}>Add a service</a>
			</div>
			<p class="note home-note">
				This build connects two services. Prowlarr gives you free-text search across your indexers
				and a grab that goes to Prowlarr's own download client. Kavita gives you a library: its
				catalogue is imported once, when you connect it. Adding either takes four things: which
				application it is, a name for it, its base URL and an API key. The connection is tested
				before anything is saved, and a service that fails its test is never stored.
			</p>
		</div>
	</div>
{/if}

<!--
	§8.5's NAMED CONFIGURATION, activated when no configured instance advertises
	LibrarySync. It is not an empty screen and it is not a loading state, and the
	copy has to say so as plainly as it says what the mode does, or the user reads
	the whole screen as a library that is coming and does not need asking for.

	⚠️ THIS COMMENT ALSO SAID "it is not a stage of setting one up", AND THAT HALF
	IS FALSE NOW. It was true while every kind the API accepted was an indexer:
	with no library-bearing kind to add, this state could not be a step toward
	anything. `internal/httpapi.serviceKinds` (internal/httpapi/services.go:50-53)
	now accepts `kavita` with role `library`, and `homeMode` ($lib/home.ts) leaves
	`search-and-grab` as soon as one configured service is not an indexer, so this
	state IS one service away from `library` and the copy below has to say which
	service. What survives of the original is the part still true: the mode is a
	real configuration to be used, not a spinner.
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
			<!--
				⚠️ `Search indexers` IS BACK, AND THE TWO ⚠️ NOTES THIS REPLACES BOTH
				RESTED ON THE SEARCH BOX ABOVE BEING ON THIS SCREEN IN THIS MODE.
				They read: that the button was "replaced rather than deleted" because
				"the search section above navigates to the same screen WITH one", and
				that `Open Services` lost `btn--primary` because "the screen now has a
				primary action — `Search indexers`, on the form above". Both premises
				died with one line: that box is now gated on `mode === 'library'`, so
				in THIS mode it is not drawn at all, and it no longer goes to Requests
				in any mode. The argument was sound and its subject left.

				So the restored control is the one the argument was made against: a
				link to Requests with no query. It is not a duplicate — there is
				nothing on this screen for it to duplicate — and it is not a second
				search box, which §8.3 would refuse. It is the route to the one thing
				§8.5 says this mode can do, on the screen whose whole text says so.

				`btn--primary` follows it, and `Open Services` gives it back for the
				reason it took it: §3.3a gives the accent to ONE control, and between
				a route to the capability and a route to the settings for it, the
				capability is what the user came for.
			-->
			<div class="empty__actions">
				<a class="btn btn--primary" href={requestsPath}>Search indexers</a>
				<a class="btn" href={servicesPath}>Open Services</a>
			</div>
			<!--
				§8.5 ends this state "Add a Sonarr, Radarr or media server to get a
				library", and what ships here is that instruction NARROWED to the one
				media server the API actually accepts. Sonarr and Radarr are still
				not named as an action, because offering an action the product
				refuses one click later is the "no invented status" failure reached
				by offering rather than by asserting.

				⚠️ WHAT SHIPPED HERE SAID THE OPPOSITE, AND IT WAS FALSE ON THE
				CENTRAL FACT. This comment read: "that instruction is NOT shipped
				here, deliberately. The API accepts one kind, so the sentence would
				be an action the product refuses one click later ... What replaces
				it says the same thing truthfully: a library is what those services
				would give you, and this build cannot connect them yet." And the
				paragraph it justified read: "A Sonarr, a Radarr or a media server
				is what would give UsArr a library to replicate, and this build does
				not accept those kinds yet. Prowlarr is the only one it can connect,
				so this is the configuration rather than a stage on the way to
				another one."

				`internal/httpapi.serviceKinds` (internal/httpapi/services.go:50-53)
				accepts TWO kinds, and the second is `kavita` with role `library`. So
				every clause above fails on the same fact: the build DOES accept a
				media server, that media server IS library-bearing, Prowlarr is NOT
				the only kind it can connect, and adding a Kavita leaves this mode
				outright, because `homeMode` ($lib/home.ts) returns `search-and-grab`
				only while EVERY configured service is an indexer. The screen was
				telling a user standing in exactly the state the remedy applies to
				that there is no remedy, and "this is the configuration rather than a
				stage on the way to another one" was the load-bearing part of the
				lie. Falsified by ADR-0041, which put Kavita in v0.1 as the sync
				core's first adapter, and by `internal/libsync`, which imports its
				catalogue.

				⚠️ AND THE REPLACEMENT IS BOUNDED ON THE OTHER SIDE, because the
				correction's own failure mode is promising a sync that does not
				exist. One adapter, one trigger, NO TIMER — `cmd/usarr/import.go` in
				those words. The import fires when a Kavita client stack is built, is
				gated on `last_full_sync_at` so it runs at most once per instance per
				database, and has no scheduler and no periodic re-read behind it. The
				copy below therefore says a first import and says it is not a running
				sync, and it names no milestone and no date for the one that is not
				built.
			-->
			<p class="note home-note">
				This build does connect a library-bearing service: Kavita, a media server UsArr replicates
				from rather than commands. Add one on Services and UsArr imports its catalogue on that first
				connect, which is what takes an install out of this mode. It is a first import and not a
				running sync: nothing re-reads the catalogue on a schedule yet. Sonarr and Radarr are not
				accepted, so they are not the way out today.
			</p>
		</div>
	</div>
{/if}

<!--
	⚠️ THE THIRD ARM'S STANDALONE EMPTY BLOCK IS GONE, AND IT WAS REPLACED
	RATHER THAN DELETED. It read "Nothing catalogued yet ... because the library
	sync is not built in this version", which was true when every kind the API
	accepted was an indexer and no importer existed. Both halves have since
	moved: `internal/libsync` imports a Kavita catalogue, and `GET
	/api/v1/library/recent` reads it. So the same state is now Block C's own
	empty state, above, three sentences from the table it is about and with the
	current reason attached instead of the old one. One region replaced one
	region; nothing was added beside it.
-->

<!--
	§9.6 rule 4's below-the-buttons paragraph, and the Block C rules that have
	nowhere else to live. Nothing here is a container, a panel or a background
	step.
-->

<style>
	/*
	 * §9.6 rule 4's below-the-buttons paragraph. `.note` is the class and the
	 * only thing this adds is the gap that keeps it off the button row; it is
	 * deliberately not a container, a panel or a background step.
	 */
	.home-note {
		margin-top: var(--space-5);
	}

	/*
	 * The search ROW — placement only. The field's own appearance moved to
	 * app.css's `.searchfield`, which the input below carries as its first
	 * class; what is left here is where the row puts it, which is the half no
	 * shared class may own.
	 *
	 * THESE FOUR LINES ARE NOT A DUPLICATE OF THE REQUESTS TOOLBAR'S FOUR even
	 * though they read identically today, and that is why they did not go with
	 * the appearance. Every one of them is flex participation: a caller that
	 * stacked this field under a label, or dropped it into a grid cell, would
	 * have to undo all four. The mockup's own `@media (max-width: 700px)` block
	 * is the recorded case — it turns `form.toolbar` into a column and resets
	 * `flex` on every child to do it.
	 *
	 * `1 1 24rem` rather than a width: the input takes the row on a phone and
	 * shares it with the button on a desktop, with no breakpoint of its own. It
	 * is 24rem here and 20rem on Requests, whose row carries a label, a select
	 * and two more buttons — the two numbers disagreed from the day they were
	 * written, which is on its own the argument against a shared basis.
	 *
	 * ⚠️ AND IT IS CAPPED, WHICH IS THE §1.5 FIX RATHER THAN A TASTE CALL.
	 * Uncapped it grew to 1,180 px at 1440 — a full-bleed input above the fold
	 * with nothing beside it, which is the hero-search bar §1.5's table sends
	 * back to the landing page. 42rem holds roughly the longest scene release
	 * name a user would actually type and leaves the row reading as a control on
	 * a screen rather than as the screen's subject. Below that width it still
	 * takes the whole row, so the phone case is unchanged.
	 *
	 * `min-width: 0` is the flex automatic-minimum-size override and stays with
	 * the row for the same reason: see `.searchfield`'s own note, which carries
	 * the measurement.
	 */
	/*
	 * THE WORDMARK — one line, one family swap, no new geometry.
	 *
	 * Every value here except `font-family` is the --text-xl pairing the type
	 * scale already defines and `.topbar__title` already uses:
	 * 20px, 28px leading, weight 600. It is written out rather than inherited
	 * because nothing else on this screen is at xl, so there is no rule to
	 * inherit from — not because the numbers are this element's own.
	 *
	 * --font-serif IS THE ONLY PLACE THIS TOKEN IS READ, and app.css's own note
	 * says so. If a second `font-family: var(--font-serif)` ever appears, the
	 * decision it belongs to is the one the owner has not made.
	 *
	 * `--fg`, EXPLICITLY, rather than by inheritance. `.section` does not set a
	 * colour, so this would land on --fg anyway; naming it is what stops the
	 * next hand from reading the absence as a slot for an accent.
	 *
	 * `letter-spacing: -0.006em` is `.topbar__title`'s value at the same size,
	 * carried across so the two renderings of 20px type in this app are tracked
	 * identically. It is optical compensation for the size, not styling.
	 *
	 * ⚠️ THIS WAS CENTRED ONCE AND THE CENTRING WAS RULED AGAINST — DO NOT PUT
	 * `text-align: center` BACK. It made the block a hero by composition, and a
	 * transparent background and no border do not make it not one: the
	 * composition is the arrangement, not the chrome. Two measurements decided
	 * it. Centred, the mark took the axis of the input-PLUS-BUTTON group
	 * (824.00) rather than the input's own centre (763.5), so it sat 60.5 px
	 * right of the field it appeared centred over; and at 1440 the content
	 * column's centre is 104 px right of the viewport centre because of the
	 * 208 px sidebar — Google centres on the window, this centred on a
	 * nav-rail-offset column. The resemblance without the geometry.
	 * check.mjs's `§13 type: no text-align:center outside dialog` therefore
	 * stands unamended, with no carve-out for this element.
	 *
	 * MARGIN IS ASYMMETRIC AND THAT IS THE ANTI-HERO CONSTRAINT, not a taste
	 * call. Nothing above it (the section head supplies its own gap) and one
	 * --space-3 below, so the wordmark sits ON the search row rather than
	 * floating above it in a band. A hero here would be vertical padding; there
	 * is none, and the block's total cost is one 28px line.
	 */
	.wordmark {
		font-family: var(--font-serif);
		font-size: var(--text-xl);
		line-height: var(--leading-xl);
		font-weight: var(--weight-xl);
		letter-spacing: -0.006em;
		color: var(--fg);
		margin: 0 0 var(--space-3);
	}

	/*
	 * ⚠️ THE ROW IS LEFT-PINNED, AND THE `justify-content: center` THAT BRIEFLY
	 * SAT HERE WAS RULED AGAINST — see the `.wordmark` rule above for the two
	 * measurements that decided it. It existed only to give a centred wordmark
	 * an axis to sit on; with the mark left-pinned it would centre the row under
	 * a mark that no longer agrees with it. The row starts at the same content
	 * edge as the heading, the scope note and every other section on this page,
	 * which is what plain flex already does — so there is no line here to
	 * express it.
	 *
	 * ⚠️ AND `max-width: 42rem` BELOW IS UNCHANGED — see its own note. It is the
	 * §1.5 hero-search cap, and it was never what this ruling was about.
	 */
	.homesearch {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}

	.homesearch__input {
		flex: 1 1 24rem;
		max-width: 42rem;
		min-width: 0;
	}

	/*
	 * THE AS-YOU-TYPE RESULTS — SPACING, AND NOTHING ELSE.
	 *
	 * No border, no background step, no shadow, no radius and no position. It
	 * is rows in the document flow directly under the field, which is what makes
	 * it an inline list rather than the floating dropdown §1.5 rules out; a
	 * panel here would need a stacking context and an outside-click contract to
	 * go with it, and would buy the user nothing the flow does not already give.
	 *
	 * ⚠️ AND THERE IS NO `transition`, NO `animation` AND NO `@keyframes` ON
	 * ANYTHING IN HERE. The list changes under a typing hand several times a
	 * second, so any entrance is either invisible or a stutter, and §7.2 Tier 0
	 * bans the fade-in by name. What replaces one answer with the next is the
	 * next answer.
	 *
	 * `--space-4` matches the gap between the scope note and Block C's own
	 * table, so the region sits at the same distance from the control above it
	 * as every other table on this screen.
	 */
	.liveresults {
		margin-top: var(--space-4);
	}

	/* A line under the box, on `.note`'s muted small type. `.note` supplies the
	 * colour, the size and the measure; this only lifts it off the field, which
	 * is placement and therefore not `.note`'s to own. */
	.livenote {
		margin-top: var(--space-3);
	}

	/* The note sits between the section head and the table, so it needs the gap
	 * on the table side rather than the paragraph spacing `.note` carries for a
	 * paragraph following a button row. It is the region's SUBTITLE and is read
	 * before the rows it governs, which is the placement design ruled for: a
	 * reader has to know what a region is before reading it, not after. Nothing
	 * here makes it one — `.note` supplies the muted colour and the small size,
	 * and the source order supplies the rest. */
	.home-grabnote {
		margin-bottom: var(--space-4);
	}

	/*
	 * THE SEPARATOR BETWEEN THE TWO RECENCY REGIONS. `--border` is the token's
	 * own stated job — "decorative divider between rows/cells" — and this is
	 * that at region scale; `--border-strong` is reserved for the boundary of an
	 * actual control, which a thematic break is not.
	 *
	 * THE MARGINS PUT IT EQUIDISTANT RATHER THAN NEAR ONE OF ITS NEIGHBOURS,
	 * which is what makes it read as belonging to the pair instead of as a
	 * decoration on the region below. `.section` carries
	 * `padding: 0 var(--space-6) var(--space-7)`, so the gap ABOVE this rule is
	 * already --space-7 from the section that precedes it, and the same value
	 * below is the whole of what this needs to be symmetric. The horizontal
	 * --space-6 is that same padding restated, because an <hr> between sections
	 * is not inside either one and would otherwise run the full width and cross
	 * the content edge every other region on this page starts at.
	 */
	.home-sectionsep {
		margin: 0 var(--space-6) var(--space-7);
		border: 0;
		border-top: 1px solid var(--border);
	}

	/*
	 * BLOCK C's Have CELL. A tier- or edition-keyed rollup renders one line per
	 * bucket, so the label and its mark share a line and the lines stack.
	 *
	 * No width is declared on either part. The COLUMN's track is declared (see
	 * RECENT_COLUMNS) and that is where ADR-0029's alignment requirement lives;
	 * inside the cell, a label that outgrows its share wraps, which is §9.1's
	 * own answer to overflow and is what the column comment means by letting the
	 * cell wrap rather than giving the track an unbounded maximum.
	 */
	.availline {
		display: flex;
		align-items: baseline;
		gap: var(--space-2);
		min-width: 0;
	}

	.availlabel {
		color: var(--fg-muted);
	}

	/*
	 * §9.5, and §17.2 states it for this exact column: chroma marks what is
	 * wrong, not what is fine. So the gap figure is the one thing in this cell
	 * that carries a hue, and the complete tick above is muted rather than
	 * green. The word is present whatever the colour does, which is §9.5's
	 * ordering (icon, text, colour) applied to a cell that has no icon.
	 */
	.availgap {
		color: var(--status-warn);
	}
</style>
