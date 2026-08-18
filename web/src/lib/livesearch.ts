/**
 * FILTER-AS-YOU-TYPE OVER YOUR OWN LIBRARY, as a plain module.
 *
 * The owner asked for it in these words: *"i feel like the search should be
 * realtime on the homepage. like filter real time type shit"*, with Navidrome's
 * instant filtering named as the feel to hit. This is the whole of the
 * behaviour behind that: the debounce, the race guard, what the region under
 * the box is allowed to claim between keystrokes, and when a screen reader is
 * told anything.
 *
 * IT IS DOM-FREE AND FRAMEWORK-FREE, like `$lib/search` and `$lib/library` and
 * for the reason those two give: `vitest.config.ts` is `environment: 'node'`
 * with no Svelte plugin, so a rule that lives inside an `{#if}` in a route is a
 * rule nothing can test. The one thing that would be untestable if it lived in
 * the component is the one thing most worth testing here — that two answers
 * landing out of order cannot paint the older one — so the whole state machine
 * is a class with an injected `search`, and the route holds nothing but markup,
 * copy and three `$state` assignments.
 *
 * ⚠️ NO NEW ENDPOINT AND NO BACKEND CHANGE. This runs against
 * `GET /api/v1/search` exactly as `routes/search` already does, and it works
 * as-you-type only because the server already matches a HALF-TYPED WORD:
 * `internal/store/searchquery.go:115` `buildFTSQueries` puts a `*` on the last
 * token (`ftsQuote(tok)+"*"`, step 4) and the trigram leg matches the whole
 * typed string as a substring whenever the longest token reaches
 * `trigramMinChars` (step 5). So `frier` finds *Frieren* on both legs, and no
 * query that is a live prefix of a real title goes blank mid-word.
 *
 * ⚠️ THE ONE INPUT THAT CANNOT MATCH IS A SINGLE ONE-CHARACTER TOKEN, and that
 * is a fact about the server rather than a guess: `searchquery.go`'s `empty()`
 * says "neither leg can run" for exactly that case, because step 4 DROPS a
 * one-character final token from the keyword leg (`if len([]rune(tok)) < 2
 * break`) and step 5 keeps it off the trigram leg. `internal/httpapi`'s
 * `docs/reference/http-api.md` §6.6 states the consequence on the wire: such a
 * query "matches nothing at all" and "answers `200` with an empty `items`". So
 * `livePlan` declines to send it. Firing it would spend a round trip to be told
 * something already known, and would then draw a *nothing matched* line under a
 * user who has typed one letter, which is the `floor` region's case instead.
 */

import { ApiError } from './api';
import { atCapacity, isCurrent, type SearchResults } from './search';

/**
 * THE DEBOUNCE, AND IT IS CHOSEN RATHER THAN INHERITED.
 *
 * The budget this spends against is `design/DESIGN-DIRECTION.md` §7.2 Tier 0 —
 * "the data is in local SQLite", target < 50 ms click-to-paint, hard fail at
 * 100 ms — and `docs/reference/http-api.md` §6, which budgets this endpoint at
 * p50 < 15 ms and p99 < 50 ms. Both numbers were written for a DISCRETE query:
 * one submit, one read. As-you-type multiplies the read count by roughly the
 * length of the query, so the debounce is what decides whether that budget is
 * spent once or ten times, and it is the only knob that does.
 *
 * THE TWO FAILURES IT SITS BETWEEN:
 *
 *   TOO LONG and the list only moves when the user STOPS, which is not what was
 *     asked for. A trailing-edge debounce at 250 ms never fires at all during
 *     continuous typing at 40 to 60 words per minute (200 to 300 ms per
 *     character), so the screen would be still for the whole burst and then
 *     jump once. That is filter-on-pause wearing the name of filter-as-you-type.
 *   TOO SHORT and every keystroke is a read with nothing coalesced, including
 *     the ones inside a fast burst that no eye could have read anyway.
 *
 * 160 ms sits under the median typist's inter-keystroke interval, so the list
 * keeps moving WHILE the user types, and above a fast burst's (a 100+ wpm run
 * is 60 to 120 ms per character), so a burst collapses into one or two reads
 * instead of ten. The wait a user actually perceives is the one after they
 * stop: 160 ms of debounce plus the endpoint's 50 ms p99 is about 210 ms, which
 * is inside the response window that reads as immediate and nowhere near the
 * one-second flow threshold.
 *
 * WHAT IT COSTS, COUNTED RATHER THAN WAVED AT. The ceiling is one read per
 * keystroke and it can never be more, because the timer restarts on each one.
 * Two things take it below that ceiling before the debounce does: the first
 * character is under `livePlan`'s floor and is never sent, and a keystroke that
 * leaves the TRIMMED query unchanged is not re-asked, which is what every space
 * key is. Measured in Chromium against the built SPA, for a ten-character query:
 *
 *   `the wire i` at 200 ms/char (60 wpm)   7 reads   1 floor, 2 spaces
 *   a ten-character single word, same rate  9 reads   1 floor
 *   any of them at 120 ms/char (100 wpm)   4 to 5    keystrokes coalesce
 *   pasted, or typed inside one window     1         one timer, one read
 *
 * ⚠️ THE 120 ms ROW IS THE ONE TO DISTRUST, and it is left standing rather than
 * silently corrected. Driven from fake timers at a PERFECTLY UNIFORM 120 ms
 * cadence the answer is **1 read, not 4 to 5**, and it has to be: a
 * trailing-edge timer restarts on every keystroke, so any uniform interval
 * below 160 ms fires exactly once, at the end. 4 to 5 is therefore reachable
 * only from a hand-typed run whose gaps sometimes exceed 160 ms, which is what
 * a real 100 wpm burst looks like — but the row does not say so, and nothing
 * here can reproduce it. Treat it as a jittered observation, not a cadence.
 * Rows 1, 2 and 4 reproduce exactly (7, 9, 1). See `REVIEW-LOG.md` WEB-02.
 *
 * Nine local reads at the endpoint's budgeted p50 of 15 ms is about 135 ms of
 * SQLite work spread over the two seconds the query took to type, on a machine
 * whose *Arrs are doing considerably more than that. The read counts above are
 * measured; that last figure is arithmetic on a budget rather than a
 * measurement on the reference hardware, and is marked so rather than left to
 * look like one.
 */
export const LIVE_DEBOUNCE_MS = 160;

/**
 * HOW MANY ROWS THE HOME REGION ASKS FOR, and it is deliberately not
 * `$lib/search`'s `SEARCH_LIMIT`.
 *
 * That one asks for the documented maximum of 100 because the Search screen is
 * the place the whole result list lives and §6.5 gives it no second page. This
 * is not that screen. It is a region under a box on Home, above Block C, and
 * every row it draws pushes the recently-added table down the page; ADR-0029's
 * per-row grid also makes rows cost real layout work on every keystroke.
 *
 * Ten is Block C's own `RECENT_LIMIT`, reused rather than re-chosen, so the two
 * tables on Home are the same height for the same reason. The full list is one
 * Enter away and the region says so when it is full: §17.4 rule 3's finding
 * from Baymard is that SILENT truncation is what makes a user believe they have
 * seen everything, and `atCapacity` is how this region knows to speak.
 */
export const LIVE_SEARCH_LIMIT = 10;

/**
 * HOW LONG AFTER THE LAST CHANGE A SCREEN READER IS TOLD ANYTHING.
 *
 * A polite live region that updates on every keystroke is a region that reads
 * out "3 results", "1 result", "12 results" over the top of somebody who is
 * still typing, and politeness only decides WHEN each of those is spoken, not
 * whether it is queued. So the count is not published on settle; it is
 * published when the user has stopped moving for long enough that a summary is
 * worth hearing, and any keystroke before then withdraws it.
 *
 * 900 ms is roughly three inter-keystroke intervals at a normal typing speed,
 * so a pause inside a word does not trigger it, and it is short enough that
 * someone who has stopped to read the result is told what is there before they
 * reach for the arrow keys.
 */
export const ANNOUNCE_SETTLE_MS = 900;

/**
 * WHAT THE REGION UNDER THE BOX IS IN A POSITION TO SAY.
 *
 * A discriminated union rather than a set of booleans, for the reason
 * `$lib/search`'s `SearchScreenState` gives: fields that must agree are fields
 * that can disagree, and "no query yet", "a query too short to run" and "a
 * query that genuinely matched nothing" are exactly the three a live region
 * gets wrong when they are separate flags.
 *
 * ⚠️ THERE IS NO `loading` MEMBER AND THERE IS NO STALE FLAG. §7.2 Tier 0 is
 * "show nothing at all. No skeleton, no spinner, no fade-in", and the reason is
 * stronger here than on a click: between two keystrokes this region is showing
 * the answer to the PREVIOUS keystroke, which is a true answer to a query the
 * user typed 200 ms ago, and swapping it for an indicator would replace
 * something true with something that says only "wait". So the previous answer
 * stays, unmarked, until the next one replaces it. That is also what stops the
 * region flickering a false *nothing found* halfway through a word: the empty
 * state is only ever drawn from an answer the server actually gave for the text
 * that is in the box.
 */
export type LiveRegion =
	/** The box is empty. Not a result of any kind, and the region is not drawn. */
	| { k: 'dormant' }
	/** One character, which `searchquery.go`'s `empty()` cannot run either leg
	 * for. Nothing was sent and nothing may be claimed about the library. */
	| { k: 'floor'; query: string }
	/** The server answered with rows, for the text that is in the box. */
	| { k: 'results'; results: SearchResults }
	/** The server answered, and had nothing. A fact about the library. */
	| { k: 'nothing'; query: string }
	/** The read failed. The library is not known to be empty; it is unknown. */
	| { k: 'error'; message: string; action: string };

/** What `type()` decided to do with one keystroke's worth of text. */
export type LivePlan =
	{ k: 'dormant' } | { k: 'floor'; query: string } | { k: 'search'; query: string };

/**
 * WHETHER THIS TEXT IS WORTH A ROUND TRIP, DECIDED BY THE SERVER'S OWN RULE.
 *
 * The three cases are `searchquery.go`'s, not a client-side minimum length
 * invented to save requests:
 *
 *   empty or whitespace   `GET /api/v1/search` answers `400 bad_request` for it
 *                         (§6.1), so sending it is a known error rather than a
 *                         cheap read.
 *   one one-character     `empty()` is true: step 4 drops a one-character final
 *   token                 token from the keyword leg and step 5 keeps every
 *                         token under three characters off the trigram leg, so
 *                         no leg runs and the answer is `200` with no items
 *                         (§6.6). Known in advance, so not asked.
 *   anything else         Sent. Two characters is enough for the keyword leg,
 *                         and `train d` still searches `train` on both legs.
 *
 * ⚠️ THE FLOOR IS ONE TOKEN OF ONE CHARACTER, NOT A QUERY OF ONE CHARACTER, and
 * the difference is `a b`: two tokens, three characters, and the keyword leg
 * runs on `"a"` because step 4 only ever drops the LAST token. Counting the
 * whole string would refuse a query the server would have answered.
 */
export function livePlan(text: string): LivePlan {
	const query = text.trim();
	if (query === '') return { k: 'dormant' };
	const tokens = query.split(/\s+/).filter((t) => t !== '');
	if (tokens.length === 1 && [...tokens[0]].length < 2) return { k: 'floor', query };
	return { k: 'search', query };
}

/**
 * The one-line summary a polite live region publishes once the user has
 * stopped. Empty for the two states there is nothing true to say about.
 *
 * The count is the rendered count, and it is qualified when the answer filled
 * the cap: §6.5 gives this endpoint no second page, so a full page means the
 * READ stopped rather than the library, and a bare number would be a claim
 * about how much there is.
 */
export function announcementFor(region: LiveRegion): string {
	switch (region.k) {
		case 'results': {
			const n = region.results.items.length;
			const noun = n === 1 ? 'match' : 'matches';
			return atCapacity(region.results)
				? `First ${n} ${noun} in your library. More on the Search screen.`
				: `${n} ${noun} in your library.`;
		}
		case 'nothing':
			return `Nothing in your library matches ${region.query}.`;
		case 'error':
			return 'Your library could not be searched.';
		default:
			return '';
	}
}

/** What the controller needs from the outside: one read, and somewhere to put
 * what it decided. */
export interface LiveSearchDeps {
	/** One search. The route passes `$lib/search`'s `fetchSearch`; a test passes
	 * a promise it controls the resolution order of. */
	search(query: string, signal: AbortSignal): Promise<SearchResults>;
	/** Called whenever the region or the announcement changed. */
	onchange(region: LiveRegion, announcement: string): void;
	/** Injected only so a test can drive time without waiting for it. */
	setTimer?(fn: () => void, ms: number): unknown;
	clearTimer?(handle: unknown): void;
}

/**
 * THE STATE MACHINE BEHIND THE BOX.
 *
 * ⚠️ TWO GUARDS, AND THEY DO DIFFERENT JOBS. Debouncing is not one of them:
 * it reduces how many reads start, and says nothing about the order they
 * finish in. Two reads are outstanding whenever a keystroke lands inside
 * another read's flight, which at a 160 ms debounce and a 50 ms p99 is
 * uncommon and, on a loaded box, entirely ordinary.
 *
 *   THE SEQUENCE NUMBER is the one that cannot be argued with. Every read takes
 *     the next integer; only the read holding the highest one may paint. An
 *     older read that resolves LAST therefore cannot repaint the region under
 *     a newer query, which is the failure mode a debounce alone leaves wide
 *     open. It also covers the case an echo check cannot: two reads of the SAME
 *     text, where both answers are `isCurrent` and only one is current.
 *   THE ABORT is the one that saves work rather than correctness. Cancelling a
 *     superseded read frees a connection against the browser's per-host limit
 *     and stops the server finishing a query nobody will look at. It is not
 *     load-bearing for ordering and must not be treated as if it were: an
 *     abort is a request to stop, and a response already on the wire arrives
 *     anyway.
 *
 * ⚠️ AN ABORTED READ REJECTS, AND THAT REJECTION IS NOT A FAILURE TO DRAW.
 * `$lib/api`'s `getJson` wraps an `AbortError` into an `ApiError` at status 0
 * because `fetch` gives it no way to tell that from an unreachable backend. The
 * sequence check catches it — an aborted read is by construction not the newest
 * — so the `catch` arm below returns before it can paint.
 *
 * ⚠️ THE ABORT FLAG IS DELIBERATELY NOT CONSULTED WHEN DECIDING WHETHER TO
 * PAINT, AND REMOVING IT FROM THAT DECISION IS A STRENGTHENING. The two lines
 * used to read `if (seq !== this.#seq || controller.signal.aborted) return;`,
 * which looks like two guards and is one guard and its shadow: for EVERY read,
 * `seq !== this.#seq` implies `controller.signal.aborted`, because `#run` and
 * `#retire` are the only things that bump `#seq` and both abort `#inflight` in
 * the same synchronous step, and `#inflight` only ever holds the newest
 * controller. So the flag was true in exactly the cases the sequence number
 * already answered, and never in any other.
 *
 * Measured rather than argued: on the shipped file at `917ddda`, deleting
 * `seq !== this.#seq` left the whole 693-test web suite green, and deleting
 * `|| controller.signal.aborted` left the whole 693-test web suite green. Each
 * leg was covered only by the other, so the pair could be drilled only as a pair and neither
 * half was provable. ⚠️ THE ORDER OF THE TWO TERMS WAS NEVER THE PROBLEM —
 * `||` already evaluated the sequence number first. Shadowing here is
 * implication, not evaluation order, and reordering would have changed
 * nothing.
 *
 * The abort keeps its real job, which is cancelling network work, and that job
 * is observable on its own: `aborts the superseded read` and `dispose cancels
 * the timer and aborts what is in flight` fail when the `abort()` calls go,
 * while the ordering tests stay green. One guard per decision, one test per
 * guard.
 */
export class LiveSearch {
	readonly #deps: LiveSearchDeps;
	readonly #setTimer: (fn: () => void, ms: number) => unknown;
	readonly #clearTimer: (handle: unknown) => void;

	/** Monotonic, and the whole of the ordering guarantee. */
	#seq = 0;
	/** The text of the most recently STARTED read, so a keystroke that leaves
	 * the trimmed query unchanged does not start a second one. Cleared by
	 * `#retire`, so emptying the box and retyping the same text asks again. */
	#asked: string | undefined;
	#inflight: AbortController | undefined;
	#debounce: unknown;
	#announce: unknown;

	#region: LiveRegion = { k: 'dormant' };
	#announcement = '';

	constructor(deps: LiveSearchDeps) {
		this.#deps = deps;
		this.#setTimer = deps.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
		this.#clearTimer = deps.clearTimer ?? ((handle) => clearTimeout(handle as never));
	}

	get region(): LiveRegion {
		return this.#region;
	}

	get announcement(): string {
		return this.#announcement;
	}

	/**
	 * ONE KEYSTROKE.
	 *
	 * The two states that need no server answer are entered IMMEDIATELY and the
	 * one that does is entered only when its answer lands. Clearing the field
	 * empties the region on the same frame rather than 160 ms later, because a
	 * region that outlives the query it answers is the one thing a user reads as
	 * broken; and a single character draws its own note at once, since the
	 * server would only have confirmed what `livePlan` already knows.
	 */
	type(text: string): void {
		this.#clearTimer(this.#debounce);
		this.#debounce = undefined;
		// The announcement is withdrawn on every keystroke and republished only
		// after the pause. Blanking it is what makes a REPEATED summary speak
		// again later: an unchanged live region says nothing.
		this.#clearTimer(this.#announce);
		this.#announce = undefined;

		const plan = livePlan(text);
		if (plan.k === 'search') {
			this.#publish(this.#region, '');
			// ⚠️ A KEYSTROKE THAT DOES NOT CHANGE THE QUERY IS NOT A NEW QUESTION.
			// `livePlan` trims, so `dune ` and `dune` are one query, and a measured
			// ten-character run of `the wire i` fired two of its nine reads on the
			// two space keys alone. Asking again would spend a round trip to be
			// told what is already on screen. An error is the one state that is
			// re-asked, because there the answer on screen is not an answer.
			if (plan.query === this.#asked && this.#region.k !== 'error') return;
			this.#debounce = this.#setTimer(() => void this.#run(plan.query), LIVE_DEBOUNCE_MS);
			return;
		}
		// Nothing is in flight that could still be wanted, so retire it: the
		// sequence bump is what makes an outstanding read unable to paint.
		this.#retire();
		this.#publish(plan.k === 'dormant' ? { k: 'dormant' } : { k: 'floor', query: plan.query }, '');
	}

	/** Cancel everything. The route calls it on teardown so a read in flight
	 * cannot resolve into a component that is gone. */
	dispose(): void {
		this.#clearTimer(this.#debounce);
		this.#clearTimer(this.#announce);
		this.#debounce = undefined;
		this.#announce = undefined;
		this.#retire();
	}

	async #run(query: string): Promise<void> {
		const seq = ++this.#seq;
		this.#asked = query;
		this.#inflight?.abort();
		const controller = new AbortController();
		this.#inflight = controller;

		try {
			const answer = await this.#deps.search(query, controller.signal);
			// ⚠️ THE SEQUENCE NUMBER IS THE WHOLE OF THE ORDERING GUARANTEE, AND IT
			// IS ALONE ON THIS LINE ON PURPOSE. `controller.signal.aborted` used to
			// sit beside it and had to be removed to make this line provable; see
			// the class note above for the measurement. `isCurrent` stays, because
			// it is NOT a proxy for this check — it is the server's own echo (§6.2
			// echoes `query` "so a client can tell a stale response from a current
			// one when the user is typing fast"), it catches a different failure
			// (an answer to text nobody typed), and it misses the one this line
			// exists for: two reads of the SAME text, where both answers echo
			// correctly and only one of them is current.
			if (seq !== this.#seq) return;
			if (!isCurrent(answer, query)) return;
			this.#settle(
				answer.items.length === 0 ? { k: 'nothing', query } : { k: 'results', results: answer }
			);
		} catch (error) {
			// The same line, and here it is the ONLY guard: there is no echo to
			// check on a rejection, so an aborted read's `ApiError` at status 0
			// would paint as a backend outage if this returned nothing.
			if (seq !== this.#seq) return;
			this.#settle(
				error instanceof ApiError
					? { k: 'error', message: error.detail, action: error.action }
					: { k: 'error', message: String(error), action: '' }
			);
		}
	}

	#retire(): void {
		this.#seq++;
		this.#asked = undefined;
		this.#inflight?.abort();
		this.#inflight = undefined;
	}

	#settle(region: LiveRegion): void {
		this.#publish(region, '');
		this.#clearTimer(this.#announce);
		this.#announce = this.#setTimer(() => {
			this.#announce = undefined;
			this.#publish(this.#region, announcementFor(this.#region));
		}, ANNOUNCE_SETTLE_MS);
	}

	#publish(region: LiveRegion, announcement: string): void {
		if (region === this.#region && announcement === this.#announcement) return;
		this.#region = region;
		this.#announcement = announcement;
		this.#deps.onchange(region, announcement);
	}
}
