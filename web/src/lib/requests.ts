/**
 * The Requests screen's pure half — ARCHITECTURE.md §17.5.
 *
 * Everything here is DOM-free and rune-free, for the same reason `list.ts` is:
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a
 * component cannot be imported into a test at all. The logic that can be wrong
 * in a way a reader would not notice — the search-type parameter mapping, the
 * fan-out sentence, the relative-time rule, and above all the grab-outcome
 * vocabulary — therefore lives in this file and is asserted in
 * `requests.test.ts`, and `+page.svelte` is left with markup.
 *
 * THE OUTCOME VOCABULARY IS THE REASON THIS FILE EXISTS. §17.5 forbids specific
 * words on this screen and the ban is not stylistic: a false "failed" invites
 * the user to grab the same 68 GB release twice, and a grab is irreversible
 * from UsArr's side. Copy that carries a rule like that belongs somewhere a
 * test can read it, not inline in a template where the next edit silently
 * reintroduces "succeeded".
 */

/* ── 1. Search types ──────────────────────────────────────────────────────── */

/**
 * One entry of the search-type selector.
 *
 * `param` is what goes on the wire as `?type=`. The five values are the five
 * `setSearchType` accepts (internal/httpapi/search.go), and a sixth would be a
 * 400 rather than a degraded search — which is deliberate upstream: Prowlarr
 * itself does NOT error on an unrecognised type, it silently falls back to a
 * basic search, and "the id filter did nothing" is very hard to diagnose from
 * the outside.
 */
export interface SearchTypeOption {
	/** Stable id, used as the `<option>` value and in the URL. */
	id: string;
	/** What the control says. Prowlarr's own five words. */
	label: string;
	/** The `type=` parameter internal/httpapi/search.go accepts. */
	param: string;
	/**
	 * True where the label reads, to anyone who uses Sonarr, Radarr or Lidarr,
	 * as a STRUCTURED mode and is not one. Prowlarr's `SearchResource` carries
	 * query, type, indexerIds, categories, limit and offset — there is no
	 * author field and no artist field — so even against an indexer advertising
	 * `book-search: [q, title, author]` the HTTP API can send free text only.
	 * §8.5 requires the UI to say so at the control rather than in an empty
	 * state the user reaches after failing.
	 */
	freeTextOnly?: boolean;
}

export const SEARCH_TYPES: readonly SearchTypeOption[] = [
	{ id: 'basic', label: 'Basic', param: 'search' },
	{ id: 'movie', label: 'Movie', param: 'movie' },
	{ id: 'tv', label: 'TV', param: 'tvsearch' },
	{ id: 'music', label: 'Music', param: 'music', freeTextOnly: true },
	{ id: 'book', label: 'Book', param: 'book', freeTextOnly: true }
] as const;

export const DEFAULT_SEARCH_TYPE = 'basic';

/** The `type=` value for a selector id. An unknown id falls back to a basic
 * search rather than sending a parameter the server will answer 400 to. */
export function searchTypeParam(id: string): string {
	return SEARCH_TYPES.find((t) => t.id === id)?.param ?? 'search';
}

/** Whether the selected type is one of the two that only LOOK structured. */
export function isFreeTextOnly(id: string): boolean {
	return SEARCH_TYPES.find((t) => t.id === id)?.freeTextOnly === true;
}

/**
 * The sentence the toolbar carries at all times. It is a constant rather than
 * template text because §8.5 makes it a requirement of the control, and because
 * the ban list below is asserted over every string this module exports.
 */
export const SEARCH_TYPE_NOTE =
	'Every search is free text matched against the release name. The type narrows what is ' +
	'searched; it does not give you an author, artist, album or title field. Book and Music look ' +
	'like structured modes and are not — Prowlarr can send a query string and nothing else, so ' +
	'Book cannot search by author and Music cannot search by artist.';

/**
 * The extra line the two free-text-only types get, and it is about the indexer
 * ecosystem rather than about UsArr (§17.5, SW-08). A music search that runs
 * correctly against healthy indexers and returns one row is indistinguishable
 * on screen from one that is broken, and the reason is that the trackers where
 * music and audiobooks live are invite-only.
 */
export const THIN_COVERAGE_NOTE =
	'403 of Prowlarr’s 543 indexer definitions are private, and the trackers that carry music and ' +
	'audiobooks are invite-only — so a stock Prowlarr returns a materially thinner list for these ' +
	'two than for a film. Adding a private indexer you already have an account on is what changes it.';

/* ── 2. The fan-out sentence ──────────────────────────────────────────────── */

/** What the fan-out line is rendered from. Real counts only: §17.5 bans a
 * progress bar over a fan-out whose remaining legs UsArr cannot time. */
export interface FanoutCounts {
	/** Indexers that answered. A failed leg is not a response. */
	answered: number;
	/** Indexers in the fan-out, where the server has said. */
	total?: number;
	/** Releases the indexers returned, before cross-indexer de-duplication. */
	releases: number;
	/** What survived de-duplication — the rows a results table would hold. */
	deduped: number;
	/** True once the closing report has landed. */
	finished: boolean;
	/**
	 * Whether a results table is actually on screen.
	 *
	 * FALSE TODAY, and this parameter is the seam rather than a stylistic
	 * choice. §17.5's sentence is "112 releases, 10 shown after
	 * de-duplication", and `shown` is a claim about the screen: while the
	 * release table is still owned by /search, nothing is shown and the word
	 * would be false. Flip this to true with the table and the sentence becomes
	 * §17.5's verbatim.
	 */
	rendered?: boolean;
}

function plural(n: number, one: string, many: string): string {
	return `${n} ${n === 1 ? one : many}`;
}

/**
 * The fan-out line — "9 of 9 indexers responded · 112 releases, 10 shown after
 * de-duplication".
 *
 * The de-duplication clause is dropped when nothing was de-duplicated: "112
 * releases, 112 after de-duplication" is a true sentence that says nothing, and
 * §9.1's rule against a column identical for every row is the same idea applied
 * to a clause.
 */
export function fanoutSummary(counts: FanoutCounts): string {
	const { answered, total, releases, deduped, finished, rendered = false } = counts;

	const head =
		total === undefined || total < 0
			? `${plural(answered, 'indexer', 'indexers')} responded`
			: `${answered} of ${total} ${total === 1 ? 'indexer' : 'indexers'} responded`;

	const count = plural(releases, 'release', 'releases');
	const tail = finished ? count : `${count} so far`;

	if (deduped === releases) return `${head} · ${tail}`;
	const shown = rendered ? 'shown after de-duplication' : 'after de-duplication';
	return `${head} · ${tail}, ${deduped} ${shown}`;
}

/* ── 3. Grab outcomes ─────────────────────────────────────────────────────── */

/**
 * How a grab row is coloured. Two roles only, and the assignment follows §9.5's
 * "chroma marks what is wrong, not what is fine": the ordinary sent row is
 * NEUTRAL, and the ambiguous one — the only row with something for the user to
 * do — carries the warn role.
 */
export type GrabOutcomeTone = 'neutral' | 'warn';

export interface GrabOutcomeCopy {
	/** The wire value this was derived from, for `data-` attributes and tests. */
	outcome: string;
	/**
	 * The chip's text. EVERY value begins with "sent", and that is the whole
	 * rule: the boundary that matters is handed-over versus not-handed-over,
	 * and every row this endpoint can return was handed over.
	 */
	label: string;
	tone: GrabOutcomeTone;
	/** ONE clause under the chip. §9.1 bans prose in a cell; the explanation
	 * lives in the block's own note, once, above the table. */
	detail: string;
	/**
	 * Always false, on every state, permanently.
	 *
	 * Retry means "do it again", and doing it again is precisely what produces
	 * two copies of a 68 GB release. It is typed as the literal `false` so a
	 * later edit that tries to switch it on for one state fails to compile
	 * rather than shipping a safe-looking button.
	 */
	offersRetry: false;
}

/**
 * The three outcome values `GET /api/v1/grabs/recent` can carry
 * (internal/httpapi/grabs.go). `not_sent` is deliberately absent: those grabs
 * write no provenance row, so they are not readable from this surface at all.
 */
export const OUTCOME_SENT = 'sent';
export const OUTCOME_SENT_UNKNOWN = 'sent_outcome_unknown';
export const OUTCOME_UNRECOGNISED = 'unknown';

/**
 * Map an outcome onto the words §17.5 permits.
 *
 * THE UNCONFIRMED ROW SITS BESIDE THE CONFIRMED ONE, VISUALLY AND VERBALLY,
 * AND NEVER BESIDE A FAILURE. Both read "sent"; the ambiguous one adds that
 * Prowlarr reported a problem after accepting the release and that UsArr cannot
 * tell whether the download is affected. Pairing them as opposites would lie in
 * both directions — implying that a 200 confirms a download and that a 500
 * means nothing happened, and neither is true. This was a real incident, not an
 * inference: the owner's book downloaded end to end in Deluge while UsArr
 * reported "Grab failed — HTTP 502".
 *
 * An unrecognised state still reads "sent", because the store's vocabulary is
 * open by design — migration 0003 ships no CHECK constraint — and the one thing
 * a provenance row always means is that the request was dispatched.
 */
export function grabOutcome(outcome: string | undefined): GrabOutcomeCopy {
	switch (outcome) {
		case OUTCOME_SENT:
			return {
				outcome: OUTCOME_SENT,
				label: 'sent',
				tone: 'neutral',
				detail: 'the client accepted it',
				offersRetry: false
			};
		case OUTCOME_SENT_UNKNOWN:
			return {
				outcome: OUTCOME_SENT_UNKNOWN,
				label: 'sent, outcome unknown',
				tone: 'warn',
				detail: 'Prowlarr reported a problem after accepting it — check your download client',
				offersRetry: false
			};
		default:
			return {
				outcome: outcome && outcome.length > 0 ? outcome : OUTCOME_UNRECOGNISED,
				label: 'sent, state not recognised',
				tone: 'warn',
				detail: 'this row is newer than this screen',
				offersRetry: false
			};
	}
}

/**
 * The line above the block that says where UsArr's knowledge stops (§17.5).
 * It is above rather than below because it governs how every row is read.
 */
export const KNOWLEDGE_STOPS_NOTE =
	'UsArr stops watching a grab the moment Prowlarr accepts it. This is what UsArr handed over and ' +
	'what Prowlarr said at the time — not what your download client has managed since.';

/**
 * The line that says which grabs are missing from the block (§17.5).
 *
 * A provenance row is written only after the request was dispatched, so a
 * rejected API key, a refused address, an open circuit breaker, a Prowlarr
 * 400/409 and a corrupt stored blob leave no row at all
 * (internal/httpapi/grabs.go). Saying so is what stops the block reading as
 * "every grab worked".
 */
export const NOT_SENT_NOTE =
	'Grabs that never got sent are not listed here yet. A rejected API key, a refused address, an ' +
	'indexer whose breaker is open or a release Prowlarr would not take leaves no record for this ' +
	'block to read, and that arm is still being built.';

/**
 * Words no copy on this screen may use, and the reason each one is banned.
 *
 *   succeeded / downloading / complete — assert that bytes are moving. UsArr
 *     deliberately stops observing at handoff, so it cannot know.
 *   done — banned AS A CONFIRMATION. Recent grabs' old wording used it and the
 *     three-state rule replaced it with "sent".
 *   failed / did not — assert that a grab did not happen, which is the one
 *     unknowable claim on the ambiguous row.
 *   retry — the action that produces two copies of a 68 GB release.
 *
 * The list is exported so the test can hold every string in this module against
 * it. That is the guard: a future edit reintroducing "succeeded" fails a test
 * rather than passing review.
 */
export const FORBIDDEN_OUTCOME_WORDS: readonly string[] = [
	'succeeded',
	'success',
	'downloading',
	'done',
	'complete',
	'failed',
	'retry'
] as const;

/* ── 4. Timestamps ────────────────────────────────────────────────────────── */

/**
 * Every user-facing timestamp carries the relative form, and past 24 hours it
 * carries a date (DESIGN-DIRECTION §9.1). One date format: `8 Aug 2026`, no
 * leading zero, always with the year.
 *
 * Hand-rolled rather than `toLocaleString`, so the rendering is the one the
 * design specifies rather than the one the browser's locale prefers, and so the
 * test asserts a value instead of whatever the CI container's ICU data returns.
 */
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

function clock(at: Date): string {
	return `${String(at.getHours()).padStart(2, '0')}:${String(at.getMinutes()).padStart(2, '0')}`;
}

/** `8 Aug 2026` — no leading zero on the day, always with the year. */
export function formatDate(at: Date): string {
	return `${at.getDate()} ${MONTHS[at.getMonth()]} ${at.getFullYear()}`;
}

/**
 * The absolute form: bare `14:07` inside 24 hours, `22:12 on 15 Aug 2026`
 * outside it. The date appears exactly when the clock alone stops identifying a
 * moment.
 */
export function formatAbsolute(at: Date, now: Date): string {
	const elapsed = now.getTime() - at.getTime();
	if (elapsed < DAY && elapsed >= -MINUTE) return clock(at);
	return `${clock(at)} on ${formatDate(at)}`;
}

/**
 * The relative form. Coarse on purpose: this block answers "did I already grab
 * this an hour ago?", and a second-resolution answer to that question is noise
 * that also re-renders every second.
 *
 * A timestamp in the future is not an error worth a special case — a clock
 * skewed by a few seconds between the server and the browser is ordinary — so
 * anything up to a minute ahead reads "just now".
 */
export function formatRelative(at: Date, now: Date): string {
	const elapsed = now.getTime() - at.getTime();
	if (elapsed < MINUTE) return 'just now';
	if (elapsed < HOUR) return `${plural(Math.floor(elapsed / MINUTE), 'minute', 'minutes')} ago`;
	if (elapsed < DAY) return `${plural(Math.floor(elapsed / HOUR), 'hour', 'hours')} ago`;
	const days = Math.floor(elapsed / DAY);
	if (days === 1) return 'yesterday';
	if (days < 30) return `${days} days ago`;
	const months = Math.floor(days / 30);
	if (months < 12) return `${plural(months, 'month', 'months')} ago`;
	return `${plural(Math.floor(days / 365), 'year', 'years')} ago`;
}

/** Both forms of one timestamp, which is what §17.3's rule requires a cell to
 * carry. An unparseable or absent value yields empty strings rather than
 * `Invalid Date`: a wrong timestamp on an irreversible action is worse than a
 * missing one. */
export function formatWhen(
	iso: string | undefined,
	now: Date
): { absolute: string; relative: string } {
	if (!iso) return { absolute: '', relative: '' };
	const at = new Date(iso);
	if (Number.isNaN(at.getTime())) return { absolute: '', relative: '' };
	return { absolute: formatAbsolute(at, now), relative: formatRelative(at, now) };
}

/**
 * `contain-intrinsic-size` for a Recent-grabs row, per density.
 *
 * 🔍 INFERENCE, marked as one. `ROW_INTRINSIC` in `list.ts` is MEASURED over
 * 2,000 rendered one-line rows; these are not measured, they are app.css's
 * `--row-h-two-line` at each density, which is the height that stylesheet
 * declares for a 13/18 title over a 12/16 sub-line — which is exactly this
 * row's shape. `auto` in front of the length means the browser replaces the
 * estimate with the row's own size once it has seen it, so the value only has
 * to be right for rows that have never been on screen. The test pins it to the
 * token values so a change to app.css that leaves this behind fails.
 */
export const RECENT_GRAB_ROW_INTRINSIC: Record<string, number> = {
	compact: 44,
	standard: 48,
	relaxed: 52
};
