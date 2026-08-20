<script lang="ts">
	/**
	 * HOME — ARCHITECTURE.md §17.2 as amended by ADR-0028, plus §17.7's first-run
	 * and error states. DESIGN-DIRECTION §8.4, §9.6, §10.
	 *
	 * ADR-0028 fixes Home at three blocks whose combined height is O(1) in the
	 * number of media types:
	 *
	 *   Block A   media-type summary    ≤6 rows           DRAWN in `library` mode
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
	 * ⚠️ BLOCK A WAS ABSENT HERE FOR AS LONG AS ITS READ WAS, AND THE READ HAS
	 * LANDED. The two blocks want different reads: `GET /api/v1/library/recent`
	 * is Block C's, and `GET /api/v1/library/facets` is Block A's — six counts
	 * over the local file, `internal/httpapi/facets.go`. Until it existed every
	 * count in Block A would have had to be invented, which DESIGN-DIRECTION
	 * §9.6 closes off by name. That constraint still SHAPES the block rather than
	 * suppressing it: the section does not render before the read lands, and it
	 * draws only the two of §17.2's five row fields the wire answers. `Have` and
	 * `Synced` are "their own aggregates and their own commit" and are therefore
	 * not drawn at all — see SUMMARY_COLUMNS.
	 *
	 * ⚠️ THE SIX ROWS ARE ALL DRAWN, AND THE ONES WITH NO SOURCE ARE §17.7's
	 * `unconfigured` STATE rather than an omission: the type, `no catalogue
	 * source connected`, the service that will populate it, the milestone it
	 * arrives in, and a link to Add. §17.2's hard rule — "a media type the user
	 * does not have is not shown AT ALL" — is satisfied by that state, which
	 * `design/DESIGN-DIRECTION.md` rule 13 says in as many words.
	 *
	 * ⚠️ AN EXEMPLAR HERE ONCE READ `Comics · no catalogue source · Kavita ·
	 * after v0.1 · Add` AND WAS FALSE ON BOTH CLAIMS, which is why the shipped
	 * split is computed in `$lib/home` off `internal/libsync` and not copied out
	 * of a document. `librarySummary` names what was measured, and names the
	 * document it disagrees with.
	 *
	 * WHAT IS STILL NOT DRAWN: §17.7's `stale` state — the non-modal banner
	 * naming a degraded instance and the time its rows were cached from. ⚠️ THIS
	 * READ *"`partial` and `stale`, which want a per-instance sync clock — a
	 * different read again"*, AND BOTH HALVES WERE WRONG AGAINST THIS SAME FILE
	 * 570 LINES BELOW. The clock is not a different read: it is
	 * `ServiceHealth.lastFullSyncAt` off GET /api/v1/services/health, which this
	 * screen already fetches for Block B and now keeps whole. And `partial` is
	 * drawn — Block A's rows carry it, as `first import running` with no number
	 * beside it, off `$lib/home`'s `countBasis`. What `stale` still wants is the
	 * BANNER, which is a rendering decision §17.7 specifies and nothing here has
	 * made: non-modal, naming the instance by the user's own name, linking to
	 * Services, and not greying the catalogue.
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
	 * AND THE FOUR THAT ARE NOT. `partial` (an import in progress) and `stale`
	 * (an instance degraded, "showing cached data from 11:47") are both claims
	 * about a catalogue and a per-instance sync clock.
	 *
	 * ⚠️ THIS USED TO READ "and neither exists". THE CLOCK HALF IS NO LONGER
	 * TRUE, and the note is corrected rather than deleted because it is the
	 * record of why the banner was not built. The per-instance clock is
	 * `ServiceHealth.lastFullSyncAt` off GET /api/v1/services/health — which
	 * this screen already fetches for Block B — and it is a specified instant
	 * rather than a plausible one: the run's START, never its finish and never a
	 * row's local write time (docs/reference/http-api.md §3.5). `null` is
	 * "never synced" and must not be rendered as a time. What is still not
	 * decided here is the banner itself: §17.7 wants it non-modal, naming the
	 * instance by the user's own name, linking to Services, and NOT greying the
	 * catalogue. Read the field off the same row as the name that goes in the
	 * sentence — the number is per instance and there is deliberately no global
	 * one. The
	 * unreachable-instance FACT is real and is reported, in Block B, where it
	 * has a source. `scope-empty` is unreachable — ⚠️ but NOT because there is
	 * no `library` table, which is what this said until migration 00005 created
	 * one and `?lib=` landed on `GET /api/v1/library` (`http-api.md` §7.3). The
	 * scope exists; Block C's endpoint is what has none. §17.2 closes that block
	 * at one table, one order and no filters, so `/library/recent` refuses the
	 * chip by design rather than by backlog, and refuses it SILENTLY: an
	 * unrecognised parameter is ignored, not rejected, so `?lib=` on this URL is
	 * 200 over the whole catalogue (`http-api.md` §1.1, and the header of
	 * `internal/httpapi/library.go`). No URL can empty a scope Home never reads.
	 * DESIGN-DIRECTION §10 lists the state as REQUIRED on Home, so it is one the
	 * design asks for and this wire cannot serve. `filtered-empty` needs a
	 * filter.
	 *
	 * BLOCK ORDER IS A, B, C ON DESKTOP AND B, A, C BELOW 760 px, which is
	 * §17.2's rule and is now two orders rather than one. ⚠️ THIS PARAGRAPH READ
	 * "with no counts to scroll past it holds at every width, so there is one
	 * order rather than two", and the premise was Block A being undrawn. §17.2's
	 * argument for promoting B on a phone is unchanged and is measured — a
	 * stacked Block A costs ~105 px per media type, which puts the block that
	 * reports a rejected API key below an 844 px fold — and it now has counts to
	 * apply to. The mechanism, and which of the two widths gets a DOM that
	 * disagrees with the screen, is at `.home-blocks` in the markup below.
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
	import {
		ApiError,
		fetchRecentGrabs,
		fetchServicesHealth,
		type RecentGrab,
		type ServicesHealth
	} from '$lib/api';
	import HaveCell from '$lib/HaveCell.svelte';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import RecentGrabs from '$lib/RecentGrabs.svelte';
	import { LOAD_MORE_PAGE_SIZE, NOTHING, type ListColumn } from '$lib/list';
	import {
		appendPage,
		cursorRejected,
		EMPTY_RECENT_FEED,
		FACETS_BODY_UNREADABLE,
		fetchLibraryFacets,
		fetchRecentPage,
		hasMore,
		mediaTypeLabel,
		nextRequest,
		NO_MEDIA_TYPE_COUNTS,
		type MediaTypeCounts,
		type RecentFeed,
		type RecentItem
	} from '$lib/library';
	import {
		attention,
		headline,
		homeMode,
		HOME_SEARCH_SCOPE_NOTE,
		librarySummary,
		summaryCaption,
		summaryCount,
		type AttentionRow,
		type HomeMode,
		type SummaryRow
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
	/**
	 * BLOCK A's COLUMNS, AND THE TWO §17.2 NAMES THAT ARE DELIBERATELY ABSENT.
	 *
	 * §17.2's row is `name · count · availability rollup · last import · see
	 * all`. `GET /api/v1/library/facets` answers the first two and says so at its
	 * own declaration: the rollup and the import time *"are their own aggregates
	 * and their own commit"*. So `Have` and `Synced` are NOT DRAWN — a value in
	 * either would be invented, which is what DESIGN-DIRECTION §9.6 closes off by
	 * name, and `Have` in particular is a specified figure (§9.5's `have / total ·
	 * N missing`) that this screen would be claiming to have computed.
	 *
	 * ⚠️ THE THIRD COLUMN IS `Status` AND NOT `Source`, WHICH IS A HONESTY CALL
	 * RATHER THAN A SYNONYM. What the cell carries is §17.7's per-type
	 * `unconfigured` STATE plus its cause and its action; it carries no service
	 * instance and cannot, because `internal/httpapi/facets.go` refuses to
	 * publish which instance a count came from — *"naming it would publish the
	 * topology of the install to every future non-owner user"*. A header reading
	 * `Source` would promise an answer three of the six rows are not allowed to
	 * give. `Have` is the one word it may not be: that is the availability
	 * rollup's name, and this column is not it.
	 *
	 * ⚠️ EVERY TRACK IS `fr`, WHICH THE DEV GUARD IN `gridTemplate()` ENFORCES.
	 * ADR-0029 makes every row its own grid, so a content-sized track resolves
	 * against its own row's contents and the header cannot agree with the body.
	 * Block B's `Action` track below documents the measured failure in detail.
	 *
	 * `Items` IS LEFT-ALIGNED AND CARRIES `.num` ON ITS DIGITS INSTEAD OF
	 * `align: 'end'`. The cell is a number PLUS a unit noun (§17.2: *"a
	 * mixed-unit column labels its unit or it is misinformation"*), so its right
	 * edge is the end of a word whose length varies row to row — right-aligning
	 * it would leave the digits ragged, which is the opposite of what the
	 * alignment buys. Left-aligned, every count starts at the same x and
	 * `tabular-nums` on the digits does the rest.
	 *
	 * THE PHONE FORK IS THE TWO-LINE ONE, AND IT IS NOT `.tbl--2up`. §17.2 asks
	 * below 760 px for a two-line row with no `Type` label, and app.css carries a
	 * `.tbl--2up` rule written for that — for §17.2's FOUR columns, splitting
	 * identity-plus-count from availability-plus-sync. That pair does not exist
	 * here: two of the four are not drawn. Its own note records an unfixed 22 px
	 * shear from the `auto` second track and says the fix needs a measurement off
	 * a real cell, and the cells it would be measured off are precisely the two
	 * this build does not have. So this list takes `stack="two-line"`, the fork
	 * `List.svelte` already stamps and Block C already uses, and `.tbl--2up`
	 * stays unreached.
	 *
	 * `Type` AND `Items` ARE LINE 1 AND `Status` IS LINE 2 — and the split is
	 * §17.2's, *"name and count on line one"*, which took a page-scoped override
	 * to reach.
	 *
	 * ⚠️ THIS PARAGRAPH CLAIMED THE SPLIT WITHOUT IT AND WAS FALSE ON THE SCREEN.
	 * It read *"`Items` IS LINE 1 BESIDE THE NAME"* and, two sentences later,
	 * *"name over count"*, which contradict each other; the render matched the
	 * second. Measured at 390 px on the shipped build at `51a9e68`: `Ebooks` at
	 * top 338 and `424 books` at top 356, both 18 px tall at weight 600 — a
	 * SECOND TITLE under the first, not a count beside a name. The cause is
	 * `.tbl--2line td[data-line='1'] { display: block }`, which gives a second
	 * line-1 column its own line. The style element at the foot of this file
	 * carries the override and what scopes it.
	 *
	 * WHY `Items` DID NOT SIMPLY MOVE TO LINE 2. `List.svelte` emits a `·` before
	 * every second-line cell except `firstSecondLine`, unconditionally and without
	 * looking at whether the cell has anything in it — so a column that is empty
	 * on half the rows may not sit on line 2 beside another, or those rows render
	 * a dangling separator. `Status` is on every row and `Items` is empty on a
	 * sourceless one, so that pairing was never available.
	 *
	 * ⚠️ AND THAT IS WHY THE OVERRIDE BINDS `Status` AS THE ONLY LINE-2 COLUMN.
	 * The override blockifies line 2 to end line 1, which is only safe while there
	 * is exactly ONE second-line cell — a second one would carry its inline `·`
	 * onto a third line. §17.2 wants this block to grow `Have` and `Synced`
	 * eventually, so the constraint is a tripwire in `home.test.ts` rather than a
	 * sentence: whoever adds a second line-2 column here fails a test that names
	 * the CSS.
	 */
	const SUMMARY_COLUMNS: ListColumn[] = [
		{ id: 'type', header: 'Type', width: 'minmax(0, 1fr)', stackLabel: false, stackLine: 1 },
		{ id: 'items', header: 'Items', width: 'minmax(0, 1.2fr)', stackLabel: false, stackLine: 1 },
		{ id: 'status', header: 'Status', width: 'minmax(0, 2.4fr)', stackLine: 2 }
	];

	/**
	 * `contain-intrinsic-size`'s estimate for a row of this list: THE TALLER OF
	 * THE TWO SHAPES, measured, and not a figure copied off another table.
	 *
	 * ⚠️ THIS READ 44 AND CLAIMED IT WAS *"the same measured figure"* AS BLOCK B's
	 * AND BLOCK C's, AND NO MEASUREMENT HAD BEEN TAKEN ON THIS LIST. Measured in
	 * Chromium at 1440×900 against the shipped compiled CSS and the real
	 * `List.svelte` row DOM, the two shapes are not one number and 44 was neither
	 * of them:
	 *
	 *   a row with a source    30 px content box (28 border box before the
	 *                          `Status` word landed on it, which is `.st`'s icon)
	 *   a sourceless row       48 px content box, carrying the service /
	 *                          milestone / Add sub-line
	 *
	 * So 44 was UNDER-reserving on half the rows rather than over-reserving on
	 * the other half, which is the error direction this constant most wants to
	 * avoid: `list.ts`'s own note records that `auto` in front of the length makes
	 * the browser discard the estimate for the row's own size the moment the row
	 * has been laid out once, so the whole of the cost is scrollbar drift on rows
	 * that have never been on screen — and over-reserving settles the scrollbar
	 * DOWNWARD as rows resolve while under-reserving pushes content away from a
	 * reader chasing it. 48 is the taller shape exactly and over-reserves the
	 * other three by 18 px.
	 */
	const ROW_INTRINSIC_SUMMARY = 48;

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
	 * THE WHOLE HEALTH RESPONSE, because Block A asks it two questions the
	 * services COUNT cannot answer.
	 *
	 * ⚠️ THIS WAS `health.services.length` AND NOTHING ELSE, and both of Block A's
	 * false sentences came out of that: with no `kind` on hand the block could
	 * only say which types the BUILD catalogues, so a Kavita-only install was told
	 * `Audiobooks · 0 audiobooks`; and with no `lastFullSyncAt` on hand it could
	 * not tell a finished import from a walk that started a minute ago, so a
	 * first-run user was told `Ebooks 0 books` for the several minutes it ran.
	 * `$lib/home` holds both derivations — `catalogueReach` and `countBasis` — so
	 * a test can read them.
	 */
	let health = $state<ServicesHealth | undefined>(undefined);

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
	 * BLOCK A's STATE.
	 *
	 * `facetsLoaded` is separate from the counts on purpose, and it is the one
	 * distinction this block cannot infer: six zeros is a REAL answer here — it
	 * is what an empty catalogue returns and what a restricted scope returns —
	 * so "all six are zero" can never mean "the read has not landed". Before it
	 * lands the section does not render at all: no skeleton, no shimmer and no
	 * zeroed table, because §9.6 bans fabricated data in a shipped surface and a
	 * placeholder row is fabricated data with rounded corners.
	 */
	let facets = $state<MediaTypeCounts>(NO_MEDIA_TYPE_COUNTS);
	let facetsLoaded = $state(false);
	let facetsError = $state('');
	/** The server's own `action`: the one thing it says fixes this. */
	let facetsAction = $state('');

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

	/* Block A's rows, the count beside its heading and the caption above it, all
	 * three derived from the same two responses so none of them can disagree with
	 * the table. `$lib/home` owns which types have a source on this install, what
	 * the six counts mean while an import is running, and why the caption is not
	 * §17.2's literal string. */
	const summaryRows = $derived(librarySummary(facets, health));
	const summaryTotals = $derived(summaryCount(summaryRows));
	const summaryNote = $derived(summaryCaption(health));

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
			const answer = await fetchServicesHealth();
			health = answer;
			mode = homeMode(answer);
			services = answer.services.length;
			rows = attention(answer.services, now);
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
		// BLOCK A, ON THE SAME CONDITION AND THE SAME ONCE-ONLY GUARD. It is asked
		// for here rather than in `onMount` because the mode is not known until
		// the health read answers, and it is asked for ONCE: `facetsLoaded` is
		// what keeps it off the 60-second timer. That timer exists because Block
		// B's clauses go stale — "retrying in 4 minutes" is wrong a minute later
		// — and a count carries no such clause, so re-reading it every minute
		// would be a repeated query for a table that changes only while the user
		// is on another screen. Recent grabs is excluded for the same reason.
		//
		// ⚠️ AND A FAILED READ IS RETRIED, WHICH `!facetsLoaded` ALONE REFUSED.
		// `facetsLoaded` is set in `finally`, so one transient 500 flipped it and
		// then gated every later attempt off: the timer went on calling `load()`
		// every minute and this line went on declining to ask again, leaving the
		// banner on screen for the rest of the session over a failure that had
		// already passed. `facetsError` is the second half of the condition and it
		// is cleared on success, so a healed endpoint heals the block on the next
		// tick and a permanently broken one is asked once a minute — the cadence
		// the health read next to it already runs at, not a spin.
		if (mode === 'library' && (!facetsLoaded || facetsError !== '')) void loadFacets();
	}

	/**
	 * BLOCK A's SIX COUNTS. A LOCAL SQLITE READ, which is what lets Home make it
	 * at all (principle 1): `internal/store/facets.go` is two statements against
	 * the local file, with pinned plans, and no *Arr, metadata provider or image
	 * fetch behind either. A degraded upstream makes these counts STALE, never
	 * an error.
	 *
	 * `facetsLoaded` is set in `finally` so a failure still ends the unrendered
	 * state: the error arm has something true to say and silence would leave the
	 * screen implying the library has nothing in it.
	 */
	async function loadFacets() {
		try {
			const counts = await fetchLibraryFacets();
			// ⚠️ A MALFORMED 200 IS A FAILURE AND IS RENDERED AS ONE. `null` is what
			// `toMediaTypeCounts` answers for a body that is not the counts
			// envelope, and it used to answer six zeros — so a renamed key on the
			// wire shipped "0 books / 0 audiobooks / 0 series" with no error
			// anywhere and the banner below unreachable. A restricted scope's zeros
			// are TRUE; a body the client could not read supports no number at all.
			if (counts === null) {
				facetsError = FACETS_BODY_UNREADABLE;
				facetsAction = '';
				return;
			}
			facets = counts;
			facetsError = '';
			facetsAction = '';
		} catch (error) {
			facetsError = error instanceof ApiError ? error.detail : String(error);
			facetsAction = error instanceof ApiError ? error.action : '';
		} finally {
			facetsLoaded = true;
		}
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
			recentRejected = cursorRejected(error, request.cursor);
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
	BLOCKS A AND B, IN ONE FLEX COLUMN BECAUSE THEIR ORDER CHANGES AT 760 px AND
	NOTHING ELSE ON THIS PAGE'S DOES.

	§17.2 fixes the desktop order at A, B, C, and then: "Below 760 px: Block A is
	a two-line row … and Block B moves above it." The reason is measured — a
	stacked Block A costs ~105 px per media type, which puts the block that
	reports a rejected API key 914 px down an 844 px viewport — so this is a rule
	about what a user reaches first, not about layout.

	⚠️ THE DOM ORDER IS THE PHONE ORDER, AND THE SWAP IS ON THE DESKTOP SIDE.
	`order` moves boxes and leaves reading order alone, so one of the two widths
	gets a DOM that disagrees with the screen and the choice is which. It is the
	desktop, because that is the width where the disagreement is harmless: both
	sections are on screen at once, neither is below a fold, and no user reaches
	one only by scrolling past the other. On a phone the whole point of the rule
	is that Block A is a screenful you must not have to get past first — and a
	screen reader gets past it the same way a scrollbar does. So the markup below
	is B then A, and the desktop rule promotes A.

	⚠️ THIS FILE'S HEADER USED TO ARGUE THAT B CAME FIRST AT EVERY WIDTH, on the
	ground that "with no counts to scroll past it holds at every width, so there
	is one order rather than two". That premise was Block A being undrawn. There
	are counts now, so there are two orders, and §17.2's is the one implemented.
-->
<div class="home-blocks">
	<!--
		BLOCK B. Hidden entirely when empty (§17.2, ADR-0028) — the green "all
		good" panel is the thing it must never become — which is why this is an
		{#if} on the row count and not a List with an empty state.
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

	<!--
		BLOCK A. THE MEDIA-TYPE SUMMARY (§17.2, ADR-0028), read from
		GET /api/v1/library/facets.

		A TABLE, NOT TILES, AND NOT A ROW OF STAT CARDS. §17.1 and
		DESIGN-DIRECTION §5.4 both settle it on the same test: a media-type
		summary's primary content is a COUNT, and a card is justified only where
		the primary content is cover art. There is no hero here, no stat banner,
		no animated counter and no box around a number.

		ALL SIX ROWS RENDER, IN THE MEDIA-TYPE ENUM'S OWN ORDER, and the ones with
		no catalogue source render §17.7's per-type `unconfigured` state rather
		than being dropped. §17.2 rejects dropping them explicitly: a Home showing
		only the types one source covers leaves "the only available inference …
		that UsArr does not do the others", which is the misreading principle 3
		exists to prevent. DESIGN-DIRECTION rule 13 is why those rows are not the
		empty section §17.1 bans — they carry a state, a cause and an action.

		WHICH TYPES THOSE ARE IS `$lib/home`'s, READ OFF `internal/libsync` AND
		NOT OFF A DOCUMENT, and it does not agree with every document. See
		`librarySummary`, which names the disagreement and what was measured.

		DRAWN ONLY IN `library` MODE, and that is principle 3 rather than an
		omission — the same gate Block C and the search box take, for the same
		reason. With no library-bearing service every one of the six rows would
		say `no catalogue source connected`, and §17.2 rules on exactly that
		shape: "Rendering six rows of which six are `no catalogue source` is not
		§17.2's screen; it is a table with no data in it, and rule 13's own bound
		— the ban is on a region that says NOTHING — does not rescue a block whose
		every row says the same nothing." An `unconfigured` install goes to the
		wizard and never to a home page at all (§17.7).

		NOTHING IS DRAWN BEFORE THE READ LANDS. No skeleton, no shimmer, no zeroed
		table: six zeros is a real answer this endpoint gives, so a zeroed
		placeholder is not a placeholder, it is a claim.
	-->
	{#if mode === 'library' && facetsLoaded}
		<section class="section" id="home-summary">
			<div class="section__head">
				<h2>Your library</h2>
				{#if !facetsError}
					<!--
						Both clauses, derived from the rows. "6 media types" alone is true
						and misleading while half of them hold no number.
					-->
					<span class="section__count num">{summaryTotals}</span>
				{/if}
			</div>

			{#if facetsError}
				<div class="banner banner--err" role="alert">
					<Icon name="x-circle" />
					<div class="banner__body">
						<div class="banner__title">Your library could not be counted</div>
						<div class="banner__text">
							This is a local read from UsArr’s own database, so it failing is not an upstream
							problem and it says nothing about how much you have. No count is shown here rather
							than a zero, because a zero is an answer this read gives for real.
						</div>
						<p class="verbatim">{facetsError}</p>
						{#if facetsAction}<p class="banner__text">{facetsAction}</p>{/if}
					</div>
				</div>
			{:else}
				<!--
					§17.2's ONE CAPTION ABOVE THE BLOCK, AND DELIBERATELY NOT §17.2's
					OWN WORDS.

					§17.2 writes the sentence out — "`Items` is the source's declared
					total from first contact, and it says so once above the block —
					'Totals reported by each service.'" — and that literal string is
					REFUSED here. `internal/store/facets.go` is `SELECT COUNT(*)` over
					local rows under the caller's access scope: it never asks a service
					for a declared total, and no field on the wire carries one. Printing
					§17.2's sentence would put a number's provenance on screen that UsArr
					has not got, which is DESIGN-DIRECTION §9.6's fabrication ban wearing
					a caption. The REQUIREMENT is kept in full — one sentence, once, above
					the block — and what it says is what this read can support, including
					the thing §17.2 wanted a caption for: that a number under an
					unfinished import is not a total. `$lib/home`'s SUMMARY_CAPTION holds
					the three sentences and the argument.
				-->
				<p class="note summary-note">{summaryNote}</p>
				<List
					label="Library by media type"
					columns={SUMMARY_COLUMNS}
					rows={summaryRows}
					key={(row: SummaryRow) => row.mediaType}
					total={summaryRows.length}
					rowIntrinsic={ROW_INTRINSIC_SUMMARY}
					stack="two-line"
					cell={summaryCellRender}
				/>
			{/if}
		</section>
	{/if}
</div>

{#snippet summaryCellRender(row: SummaryRow, column: ListColumn)}
	{#if column.id === 'type'}
		<!--
			THE NAME IS THE LINK, AND ONLY WHERE THERE IS SOMETHING BEHIND IT.
			§17.2's row ends in "see all"; a separate column for it would be a
			second control on every row of a six-row list, so the identity carries
			it — which is what the mockup does too. A sourceless row's name is
			plain text: the screen behind it is an empty grid, and a link to an
			empty table is the thing this row exists instead of.
		-->
		{#if row.catalogued}
			<a class="cell-title trunc" href={resolve('/library/[type]', { type: row.mediaType })}>
				{row.label}
			</a>
		{:else}
			<span class="cell-title trunc">{row.label}</span>
		{/if}
	{:else if column.id === 'items'}
		<!--
			THE UNIT NOUN IS PART OF THE CELL (§17.2). A sourceless row renders
			NOTHING here rather than a `0` or a dash: its count is zero on the wire
			like every other type nothing has catalogued, and "0 films" on an
			install where nothing has ever looked for a film is a claim about the
			library rather than about the pipeline. The next cell says what is
			actually the case.
		-->
		{#if row.items}<span class="num">{row.items}</span>{/if}
	{:else}
		<!--
			§17.7's PER-TYPE STATE, AND EVERY ROW HAS ONE.

			⚠️ THIS ARM WAS `{:else if !row.catalogued}` AND THE `Status` COLUMN WAS
			BLANK ON EVERY ROW THAT HAD A STATUS. A header promising a state above an
			empty box reads as *status unknown*, not as *status fine*, on exactly the
			three rows the screen is most confident about. §17.7 has an `ok` state and
			the services read says which rows are in it, so the column delivers what
			its header promises rather than being renamed down to what it managed.

			The vocabulary is `$lib/home`'s `SUMMARY_STATE`, carried on the row, so
			there is no second copy of the mapping in a template.

			⚠️ ALL THREE TAKE THE SAME GREY, INCLUDING `ok`, AND THAT IS THE DESIGN'S
			RULE RATHER THAN AN OVERSIGHT. Nothing here is broken and nothing failed —
			a type with no source and a walk still running are both ordinary — so
			neither an error nor a warning tone is available; and `st--ok`'s green on
			the three healthy rows would turn Block A into the reassurance panel
			DESIGN-DIRECTION §9.5 rules out, *"chroma marks what is wrong, not what is
			fine"*. `.st--none`'s own note says grey is load-bearing for exactly this.
			The icon is what separates the three, which is `.st`'s rule too: icon and
			text together, never colour alone. `check` and `dash-circle` are two of
			`Icon.svelte`'s six names; no seventh is minted for a distinction the
			words already carry.

			The link is to Services rather than a second copy of the add flow:
			§17.3's rule that a problem is stated canonically once per screen, and
			there is one place a service gets added.
		-->
		<span class="st st--none">
			<Icon name={row.state === 'ok' ? 'check' : 'dash-circle'} size="sm" />
			{row.status}
		</span>
		{#if !row.catalogued}
			<div class="cell-sub">
				{row.service} · {row.milestone} · <a href={servicesPath}>Add</a>
			</div>
		{/if}
	{/if}
{/snippet}

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
			§17.2's Have column, and every decision in it belongs to
			`$lib/library.haveCell` and to `$lib/HaveCell.svelte` — the tick that must
			never fire on `total: null`, the gap figure that carries §9.5's warn role,
			and the row nothing has counted, which carries no glyph at all. Search
			renders the SAME component: the two screens drew this cell from two copies
			of one chain, and the copies had already come apart in their styling.
		-->
		<HaveCell {item} />
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
	 * §17.2's caption above Block A — placement only. `.note` is the class and
	 * the whole of the appearance; this adds the gap that keeps it off the
	 * section rule above it and off the table below it. `.section__head` already
	 * carries the space above, so only the space below is written here.
	 */
	.summary-note {
		margin-bottom: var(--space-4);
	}

	/*
	 * BLOCKS A AND B, AND THE ONLY THING THIS CONTAINER DOES IS MAKE `order`
	 * AVAILABLE TO THEM. It is not a panel, a card or a background step: no
	 * padding, no border, no radius and no background, so the two sections sit
	 * exactly where they sat as siblings of `.main__inner`.
	 *
	 * `column` with no `gap`, deliberately. `.section` carries its own
	 * `padding: 0 var(--space-6) var(--space-7)`, so the spacing between these
	 * two is already the spacing between every other pair of sections on this
	 * page; a gap here would make this one pair further apart than the rest and
	 * the region would read as a group, which it is not.
	 *
	 * The default order is the markup order, B then A, which is the phone's.
	 */
	.home-blocks {
		display: flex;
		flex-direction: column;
	}

	/*
	 * ABOVE 760 px, §17.2's block order: A, B, C. The breakpoint is the list
	 * primitive's own — `app.css`'s single `@media (max-width: 760px)` is where
	 * every table on this screen becomes a stack — so the two forks change at one
	 * width rather than at two, and `min-width: 761px` is that same boundary
	 * written from the other side.
	 *
	 * ⚠️ ONE DECLARATION, ON BLOCK A, AND NOT A PAIR. Giving B an explicit
	 * `order: 2` as well would be two values that must agree; a single negative
	 * order on A promotes it past any sibling and cannot disagree with anything.
	 */
	@media (min-width: 761px) {
		#home-summary {
			order: -1;
		}
	}

	/*
	 * §17.2's PHONE ROW: "name and count on line one".
	 *
	 * ⚠️ THE SHARED FORK CANNOT DO THIS AND MUST NOT BE MADE TO. `.tbl--2line`
	 * gives every `data-line="1"` cell `display: block`, so a second line-1 column
	 * starts its own line — `424 books` rendered at 13px semibold on a line of its
	 * own, which is a second title rather than a count beside a name. The obvious
	 * repair, moving `Items` to line 2, is closed off by `List.svelte`: the fork
	 * emits a `·` before every line-2 cell except `firstSecondLine`, without
	 * looking at whether the cell has anything in it, and `Items` is empty on a
	 * sourceless row while `Status` is empty on none — so the pair would render a
	 * dangling separator on half the rows.
	 *
	 * So the two line-1 cells go inline and the line-2 cell takes the block that
	 * ends the line. THIS IS SCOPED TO `#home-summary` AND MUST STAY THERE: the
	 * `display: block` on line 2 is only safe on a list with exactly ONE
	 * second-line column, because `.stacksep` is `display: inline` and a
	 * blockified second cell would carry its `·` onto a third line. Block C and
	 * every other `stack="two-line"` list has two, and none of them is touched by
	 * an id selector on this section.
	 *
	 * `:global()` because the cells are `List.svelte`'s, not this file's; the id
	 * is what keeps the reach to this one table.
	 */
	@media (max-width: 760px) {
		#home-summary :global(.tbl--2line td[data-line='1']) {
			display: inline;
		}

		/* The gap between the name and its count. Not a `gap` — there is no flex
		 * or grid container on this line any more — and not markup whitespace,
		 * which the compiler is free to collapse away. */
		#home-summary :global(.tbl--2line td[data-col='type']) {
			padding-right: var(--space-3);
		}

		#home-summary :global(.tbl--2line td[data-col='type'] .trunc),
		#home-summary :global(.tbl--2line td[data-col='items'] .num) {
			display: inline-block;
			vertical-align: top;
		}

		#home-summary :global(.tbl--2line td[data-line='2']) {
			display: block;
		}
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
</style>
