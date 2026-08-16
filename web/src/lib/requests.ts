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
	/**
	 * ONE clause under the chip, AND ONLY WHERE IT SAYS SOMETHING THE LABEL DOES
	 * NOT. §9.1 bans prose in a cell and drops a value identical for every row of
	 * a group — "state the fact once, in the group header" — and a clause is a
	 * column one cell wide. The confirmed state therefore carries none: "the
	 * client accepted it" under every `sent` chip was the chip restated, on every
	 * confirmed row, at the cost of a second line each. The fact it carried is
	 * still on screen once, above the table, in `KNOWLEDGE_STOPS_NOTE` — which
	 * opens by naming the moment Prowlarr accepts a grab.
	 *
	 * The two states that keep a clause keep it because it is an INSTRUCTION
	 * rather than a restatement: where to look when the outcome is unknown, and
	 * why this screen cannot read a row it has never heard of. Both are facts the
	 * chip does not carry, so §9.1's test — delete this clause, does the user
	 * lose a fact they can act on? — answers yes for those two and no for `sent`.
	 */
	detail?: string;
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
			// No detail: see GrabOutcomeCopy.detail. The label is the whole fact,
			// and the block's own note above the table carries the rest once.
			return {
				outcome: OUTCOME_SENT,
				label: 'sent',
				tone: 'neutral',
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

/* ── 5. The release row's cells ───────────────────────────────────────────── */

/**
 * ⚠️ THE INDEXER-FLAG VOCABULARY IS OPEN, PERMANENTLY. REVIEW-LOG SW-18,
 * checked against Prowlarr `develop` rather than inferred.
 *
 * `IndexerFlag` is a CLASS, not an enum (`src/NzbDrone.Core/Indexers/
 * IndexerFlag.cs`): seven statics, every one written `new(...)`, and
 * `PassThePopcornFlag : IndexerFlag` subclasses it to add `golden` and
 * `approved`. Nine today and open forever. A first attempt at this column
 * matched a CLOSED SET OF SEVEN and would have dropped `golden` today and every
 * future indexer's flags after it — INVISIBLY, the row simply showing fewer
 * chips than the indexer sent. So a chip renders whatever string arrives and
 * nothing here is an allowlist.
 *
 * `repack` and `proper` are NOT indexer flags and must never be drawn as ones:
 * both are release-title qualifiers the *Arrs parse out of the NAME. This slot
 * is fed by `indexer_flags` alone — Go field `Flags`, JSON tag `indexer_flags` —
 * and never by parsing a title.
 *
 * `EMPHASISED_FLAGS` is a two-name EMPHASIS test, not a filter. Those two alone
 * are DERIVED rather than indexer-supplied — `TorznabRssParser.GetFlags` sets
 * them from `downloadFactor == 0.0` and `== 0.5` — and they alone change what a
 * download costs a ratio. Everything else renders as a plain chip.
 */
export const EMPHASISED_FLAGS: readonly string[] = ['freeleech', 'halfleech'] as const;

export function isEmphasisedFlag(flag: string): boolean {
	// Case- and separator-insensitive, because the same flag arrives as
	// `freeleech`, `FreeLeech` and `Free Leech` from different definitions.
	return EMPHASISED_FLAGS.includes(flag.toLowerCase().replace(/[^a-z]/g, ''));
}

/**
 * ⚠️ AN EMPTY FLAGS CELL MEANS UNKNOWN, NEVER "NOT FREELEECH", AND THE TWO
 * ABSENCES ARE DIFFERENT FACTS.
 *
 * `GetFlags` runs only inside `if (torrentInfo != null)` and `NewznabRssParser`
 * never touches a flag, so a usenet result carries no flag field at all — the
 * indexer was never asked. A torrent that reports an empty list WAS asked and
 * said none. And because the two derived flags are exact-equality tests on a
 * double defaulting to `1`, a 25% or 75% promotion produces nothing either way.
 *
 * `None` is banned for both, for the same reason `list.ts` bans it: it asserts
 * a value where UsArr has an absence.
 */
export const FLAGS_NOT_REPORTED = 'Not reported';
export const FLAGS_NONE_REPORTED = 'None reported';

/** Which absence a row's empty flag cell is. Usenet was never asked; a torrent
 * with nothing to say was. */
export function flagsAbsence(protocol: string | undefined): string {
	return protocol === 'usenet' ? FLAGS_NOT_REPORTED : FLAGS_NONE_REPORTED;
}

/**
 * The Category cell's words.
 *
 * It reads the server's derived `type:` and `format:` tags rather than mapping
 * the raw ids here, and that is not laziness: `mapping.MediaType` runs TWO
 * passes so that `[3000, 3030]` — an audiobook, which Prowlarr emits
 * parent-first — comes out `type:book · audiobook` rather than `type:music`,
 * which is precisely the mistake category 3030 exists to prevent. One rule,
 * server-side. The raw ids are the fallback, because a category UsArr has no
 * `type:` for is still a category the indexer filed the release under.
 */
export function categoryLabel(tags: readonly string[], categories: readonly number[]): string {
	const type = tags.find((t) => t.startsWith('type:'))?.slice(5) ?? '';
	const format = tags.find((t) => t.startsWith('format:'))?.slice(7) ?? '';
	if (type && format) return `${type} · ${format}`;
	if (type) return type;
	if (categories.length > 0) return categories.join(', ');
	return '';
}

/* ── 6. The grab window ───────────────────────────────────────────────────── */

/**
 * ⚠️ PROWLARR'S GRAB CACHE IS A NON-ROLLING 30 MINUTES, AND §17.5 MAKES THIS A
 * REQUIREMENT RATHER THAN A NICETY.
 *
 * The screen states that an expired release is never offered as grabbable,
 * which is only TRUE IF THE CLIENT ACTS ON IT. Otherwise a user who read
 * "closes in 18 minutes", worked through the list and pressed Grab receives a
 * 400 they were promised could not happen.
 *
 * ⚠️ THE ANNOUNCEMENT STEPS ARE 5, 2 AND 1 MINUTES, AND THE LIVE REGION'S TEXT
 * CHANGES ONLY AT THEM. A `role="status"` whose content ticks every second
 * announces every second, which is how a screen-reader user ends up turning the
 * screen off. So `notice` is the SAME STRING for every instant inside a step,
 * and the empty string above the first one — an unchanged live region announces
 * nothing at all, which is the behaviour being bought.
 */
export const GRAB_WINDOW_STEPS_MINUTES = [5, 2, 1] as const;

export interface GrabWindow {
	/** True once nothing on screen can still be grabbed. */
	expired: boolean;
	/** Whole minutes left, floored. -1 when there is no expiry to measure. */
	minutesLeft: number;
	/** The always-visible reading. NOT a live region — see `notice`. */
	label: string;
	/** The `role="status"` text. Empty above five minutes, by design. */
	notice: string;
}

export const GRAB_WINDOW_EXPIRED_NOTICE =
	'The grab window has closed. Prowlarr keeps a listing grabbable for 30 minutes and these are ' +
	'past it, so search again to get ones that can still be sent.';

/** The per-row note beside a grab control that has gone `aria-disabled`. The
 * sentence explaining it is above the table, once — §9.1 bans a clause that is
 * identical on every row of a state. */
export const GRAB_ROW_STALE_NOTE = 'listing went stale';

export function grabWindow(expiresAt: string | undefined, now: Date): GrabWindow {
	// No expiry on the wire is not an expiry of zero. Saying nothing is the only
	// honest reading, and the grab control stays live.
	if (!expiresAt) return { expired: false, minutesLeft: -1, label: '', notice: '' };
	const at = new Date(expiresAt);
	if (Number.isNaN(at.getTime())) return { expired: false, minutesLeft: -1, label: '', notice: '' };

	const msLeft = at.getTime() - now.getTime();
	if (msLeft <= 0) {
		return {
			expired: true,
			minutesLeft: 0,
			label: 'grab window closed',
			notice: GRAB_WINDOW_EXPIRED_NOTICE
		};
	}

	const minutesLeft = Math.floor(msLeft / MINUTE);
	const label =
		minutesLeft < 1
			? 'grab window closes in under a minute'
			: `grab window closes in ${plural(minutesLeft, 'minute', 'minutes')}`;

	// The step this instant falls in, or none. Constant across a whole step, so
	// the live region stays quiet between announcements.
	let notice = '';
	for (const step of GRAB_WINDOW_STEPS_MINUTES) {
		if (minutesLeft < step) {
			notice =
				step === 1
					? 'Under a minute left to grab these releases.'
					: `${step} minutes left to grab these releases.`;
		}
	}
	return { expired: false, minutesLeft, label, notice };
}

/* ── 7. A grab, as it happens ─────────────────────────────────────────────── */

/**
 * The live grab's states, mapped off the SERVER'S ERROR CODE and never off its
 * prose (internal/httpapi/errorcodes.go).
 *
 * This is §17.5's three-state rule at the moment the button is pressed. It is
 * written separately from `grabOutcome` above, which applies the same rule to a
 * STORED row: the inputs are different — an HTTP error code against a
 * provenance value — and one function over a union of the two would hide which
 * mapping produced a label.
 *
 * ⚠️ `grab_outcome_unknown` GETS NO BUTTON, AND THAT IS THE STATE'S WHOLE POINT.
 * The release reached Prowlarr; what the download client did with it is
 * unknown, and the owner's own book downloaded end to end in Deluge while UsArr
 * reported an error. The only button that would fit is one that sends the
 * release again, and sending it again is exactly what produces two copies of a
 * 68 GB release. The server's own action — "Check your download client" —
 * renders as TEXT, because that is where the truth actually lives.
 *
 * ⚠️ AND THERE IS NO RETRY ON ANY STATE, WHICH NARROWS WHAT THE PREVIOUS SCREEN
 * DID. It offered Retry on `grab_failed`, on the reasoning that the code
 * asserts nothing was sent so a second press cannot duplicate anything. True,
 * and still not worth a button: after the code thread narrowed `grab_failed`,
 * everything under it is a bad API key, an open circuit breaker, an SSRF
 * refusal, a Prowlarr 400 or 409, a corrupt stored blob, or a body Prowlarr
 * would not bind. NOT ONE OF THOSE IS FIXED BY PRESSING THE SAME BUTTON AGAIN —
 * the transient cases that a retry would help are precisely `ErrServer`,
 * `ErrTimeout` and `Canceled`, and those are the ambiguous code that must never
 * offer one. So the honest set of actions is `Search again` where the listing
 * went stale, and the server's own sentence everywhere else.
 */
export type LiveGrabTone = 'neutral' | 'warn' | 'err';

export interface LiveGrabCopy {
	/** The chip's words. Both handed-over states begin with "Sent". */
	label: string;
	tone: LiveGrabTone;
	/** Whether the row may offer `Search again` — true only where nothing was
	 * sent AND the listing is genuinely gone from Prowlarr's cache, so a fresh
	 * search is the one action that can work. */
	offersSearchAgain: boolean;
	/** Always false, on every state, permanently. Typed as the literal so an
	 * edit that switches it on for one state fails to compile rather than
	 * shipping a safe-looking button. */
	offersRetry: false;
}

/** The codes that mean "this listing is gone from Prowlarr's 30-minute cache".
 * Retrying the same opaque release id returns the same 4xx for ever. */
export const RESEARCHABLE_CODES: readonly string[] = [
	'expired',
	'no_longer_offered',
	'search_failed'
] as const;

export const CODE_OUTCOME_UNKNOWN = 'grab_outcome_unknown';

export const LIVE_GRAB_SENT_LABEL = 'Sent to Prowlarr';
export const LIVE_GRAB_UNKNOWN_LABEL = 'Sent, outcome unknown';
export const LIVE_GRAB_STALE_LABEL = 'Not sent, the listing went stale';
export const LIVE_GRAB_NOT_SENT_LABEL = 'Not sent';

export function liveGrabCopy(code: string): LiveGrabCopy {
	if (code === CODE_OUTCOME_UNKNOWN) {
		return {
			label: LIVE_GRAB_UNKNOWN_LABEL,
			tone: 'warn',
			offersSearchAgain: false,
			offersRetry: false
		};
	}
	if (RESEARCHABLE_CODES.includes(code)) {
		return {
			label: LIVE_GRAB_STALE_LABEL,
			tone: 'err',
			offersSearchAgain: true,
			offersRetry: false
		};
	}
	// §17.5's state 3: nothing was sent, so nothing is running. The verdict is
	// on the request rather than on the release — the server's own sentence,
	// rendered verbatim beside this, says why.
	return {
		label: LIVE_GRAB_NOT_SENT_LABEL,
		tone: 'err',
		offersSearchAgain: false,
		offersRetry: false
	};
}

/* ── 8. The frozen order's one control ────────────────────────────────────── */

/**
 * The sentence beside ADR-0038's re-sort control, so the freeze is explained
 * rather than merely felt.
 *
 * The control's own label lives in `$lib/frozenorder.svelte`, because it
 * carries a count the engine owns. This is the standing explanation, and it is
 * here rather than there because it is copy and this is the file a copy test
 * can read.
 */
export const FROZEN_ORDER_NOTE =
	'The order is held while you are pointing at these results or focused inside them, so a release ' +
	'cannot move out from under the button you are aiming at.';

/* ── 9. Row heights ───────────────────────────────────────────────────────── */

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

/**
 * The same statistic for a RELEASE RESULT row, which is the taller of the two.
 *
 * MEASURED, and not by this thread: `list.ts`'s ROW_INTRINSIC comment records
 * `scripts/list-bench.mjs` rendering the release row the harness draws — chips,
 * a button, a checkbox and a `<select>` — at 45 / 49 / 53 px content box across
 * the three densities, i.e. 1.6x the one-line default. That is this row's
 * shape, so those are the numbers rather than a fresh guess. `auto` in front of
 * the length still means the browser replaces the estimate with the row's own
 * size once it has seen one.
 */
export const RELEASE_ROW_INTRINSIC: Record<string, number> = {
	compact: 45,
	standard: 49,
	relaxed: 53
};
