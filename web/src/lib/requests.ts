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
 * How a grab row is coloured, and the assignment follows §9.5's "chroma marks
 * what is wrong, not what is fine": the ordinary sent row is NEUTRAL, the
 * ambiguous one carries WARN, and the row that never left carries ERR.
 *
 * ⚠️ `err` IS THE THIRD STATE'S ROLE AND NOT A THIRD DEGREE OF THE SAME AXIS.
 * §17.5's three states are not "fine, worrying, bad": the first two are the SAME
 * epistemic state about the download — handed over, differing only in whether an
 * error came back afterwards — and the third is categorically different, because
 * nothing was handed over and nothing can be running. So the tone gap between
 * `warn` and `err` is doing the work §17.5 asks of it: state 2 sits beside state
 * 1, and state 3 sits beside neither.
 */
export type GrabOutcomeTone = 'neutral' | 'warn' | 'err';

/**
 * The one action a Recent-grabs row may offer, and `none` is a first-class value
 * rather than an absence — the same construction `CorrelatedAction` uses, for
 * the same §17.5 reason: where the correct action is not something UsArr can
 * offer, naming the non-action beats offering a fake one.
 *
 * ⚠️ THERE IS NO MEMBER THAT GRABS AGAIN, AND THERE WILL NOT BE. `search-again`
 * runs a fresh fan-out; it does not re-send an irreversible grab, and it is
 * offered only where a fresh listing is the thing that was missing.
 * `open-services` is navigation. Neither posts anything.
 */
export type GrabRowAction = 'none' | 'search-again' | 'open-services';

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
	 * The states that keep a clause keep it because it is an INSTRUCTION rather
	 * than a restatement: where to look when the outcome is unknown, why this
	 * screen cannot read a row it has never heard of, and — on the not-sent arm —
	 * WHICH of a dozen conditions stopped it, which is the only thing that
	 * distinguishes one such row from another. Both are facts the chip does not
	 * carry, so §9.1's test — delete this clause, does the user lose a fact they
	 * can act on? — answers yes for those and no for `sent`.
	 */
	detail?: string;
	/**
	 * The sentence that names what this screen CANNOT do, per §17.5's "naming the
	 * non-action beats offering a fake one".
	 *
	 * Empty in exactly two situations and no others: where the offered action
	 * genuinely resolves the condition (saying "there is nothing you can do"
	 * beside a button that works is its own dishonesty), and on the two
	 * handed-over states, whose row has nothing to fix because nothing is known
	 * to be wrong with it.
	 */
	nonAction?: string;
	/** What the row may offer. `none` on every handed-over state, permanently:
	 * the release is with the download client and this screen is not where it is
	 * resolved. */
	action: GrabRowAction;
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
 * The FOUR outcome values `GET /api/v1/grabs/recent` can carry
 * (internal/httpapi/grabs.go).
 *
 * ⚠️ `not_sent` DELIBERATELY DOES NOT BEGIN WITH "sent", AND NOTHING MAY MATCH
 * THESE BY PREFIX. `'not_sent'.startsWith('sent')` is false, but the near-miss
 * runs the other way — `startsWith` on the string `sent` was the obvious way to
 * ask "was this handed over?" while there were only three values, and a
 * `.includes('sent')` written in the same spirit answers TRUE for the one state
 * where nothing was handed over. Every comparison in this file is a full-string
 * equality for that reason, and the switch below is exhaustive rather than
 * ordered.
 */
export const OUTCOME_SENT = 'sent';
export const OUTCOME_SENT_UNKNOWN = 'sent_outcome_unknown';
export const OUTCOME_NOT_SENT = 'not_sent';
export const OUTCOME_UNRECOGNISED = 'unknown';

/**
 * What the not-sent arm carries beyond the outcome itself: the server's error
 * code, and whether the row has a release name at all.
 *
 * `hasTitle` is here rather than left to the template because it changes the
 * COPY and not only the markup. `release_title` may be `""` on this arm — the
 * candidate had been swept and the name is genuinely unknown — and a row with no
 * name has nothing to search for, so "Search again" there would be a button that
 * cannot be aimed. The condition decides the sentence as well as the control,
 * and a sentence promising an action that is not rendered is the fake action
 * §17.5 bans wearing different clothes.
 */
export interface GrabOutcomeInput {
	/** `error_code` off the wire. Present only on the not-sent arm. */
	errorCode?: string;
	/** Whether `release_title` is a non-empty string. */
	hasTitle?: boolean;
}

/**
 * ⚠️ THE NOT-SENT COPY IS KEYED ON THE ERROR CODE AND NEVER ON PROSE, BECAUSE
 * THERE IS NO PROSE TO KEY ON AND THERE WILL NOT BE.
 *
 * `internal/httpapi/grabs.go` ships `error_code` and deliberately withholds the
 * message: the message is assembled from an upstream error string and would
 * force this endpoint's "no URL, no passkey, no key" response guard open to
 * carry it. The user was shown that sentence, verbatim, at the moment of the
 * grab; this block is the history, and the history gets the code.
 *
 * ⚠️ AND THE TABLE IS THE ACTION RULE, NOT A GLOSSARY. §17.5: a failure offers
 * the action that can succeed and never one that structurally cannot. Three
 * questions decide each row, in this order:
 *
 *  1 **What stopped it**, in one clause — the only thing that separates one
 *    not-sent row from another, since the chip is the same on all of them.
 *  2 **Is there an action that genuinely helps?** `search-again` is offered only
 *    where a fresh LISTING is the thing that was missing — Prowlarr's 30-minute
 *    cache had dropped it, the recovery search did not finish, or the candidate
 *    row was gone. `open-services` is offered only where UsArr's own service
 *    configuration is the thing at fault, which is the one class of these that
 *    UsArr's own screens can change.
 *  3 **Where there is none, say so.** `grab_failed` is the case this rule was
 *    written for: it is the unclassified remainder, so the row cannot say which
 *    of a rejected credential, a refused address, a paused indexer or a request
 *    Prowlarr would not take stopped it — and not one of those is moved by
 *    asking again. §17.5's own words: a screen that reasons its way to a
 *    plausible diagnosis and then hands the user a button that cannot act on it
 *    is worse than one that says what it can see and stops.
 *
 * 🔍 THE CODES ARE READ OFF internal/httpapi/errorcodes.go AND THE GRAB PATH
 * THAT EMITS THEM (grab.go's grabError and auditNotSent, search.go's
 * searcherFor). `not_configured` is in the table and not in §17.5's enumeration
 * because searcherFor emits it and auditNotSent records whatever it emits;
 * carrying it costs one row and its absence would have been an unlabelled fall
 * to the unrecognised state.
 */
interface NotSentCopy {
	detail: string;
	nonAction: string;
	action: GrabRowAction;
}

const NOT_SENT_BY_CODE: Record<string, NotSentCopy> = {
	// Prowlarr's grab cache is a non-rolling 30 minutes and this one was past it.
	// §17.5 is explicit that this reads as EXPIRED and offers Search again —
	// never as a rejection, which would read as "the tracker refused you".
	expired: {
		detail: 'the listing had expired — Prowlarr keeps a release grabbable for 30 minutes',
		nonAction: '',
		action: 'search-again'
	},
	// The one path where the re-search RAN and came back without the release, so
	// "the indexer no longer offers it" is a claim the evidence supports. Searching
	// again does not FIX that — it is how you find out whether it has come back —
	// so the non-action is stated even though an action is offered.
	no_longer_offered: {
		detail: 'UsArr looked again and the indexer no longer offered this release',
		nonAction:
			'whether it comes back is the indexer’s call, so searching again is only how you find out',
		action: 'search-again'
	},
	// The cache had dropped it AND the recovery search did not land. Nothing was
	// sent either way, and running the search again is the whole of the remedy.
	search_failed: {
		detail:
			'Prowlarr had dropped the listing, and the search UsArr ran to recover it did not finish',
		nonAction: '',
		action: 'search-again'
	},
	// The candidate row was gone — swept after its TTL, or outside this account's
	// scope. A fresh search re-lists it under a new id, which is the only way back
	// to a grabbable listing, so it is offered WHERE THERE IS A NAME TO SEARCH FOR.
	// The nameless variant is handled below rather than here.
	not_found: {
		detail: 'the listing was gone when you pressed Grab — swept, or outside this account’s reach',
		nonAction: '',
		action: 'search-again'
	},
	// UsArr's own configuration, and the one class of these its own screens change.
	no_indexer_service: {
		detail: 'UsArr had no enabled indexer service to send the grab through',
		nonAction: '',
		action: 'open-services'
	},
	// The service exists and could not be opened. Services is honest here BECAUSE
	// it shows reachability now rather than then — which is also why the row says
	// so instead of implying the state has not moved since.
	service_unavailable: {
		detail: 'UsArr could not open the indexer service that owns this listing',
		nonAction: 'this row is what happened then, and Services shows what UsArr can reach now',
		action: 'open-services'
	},
	not_configured: {
		detail: 'this build has no release-search service wired in',
		nonAction:
			'that is a property of the binary rather than of your setup, so nothing here moves it',
		action: 'none'
	},
	// Prowlarr-side settings. Real, nameable, and not UsArr's to change — so the
	// row names where the switch lives and offers no button, per §17.5's rule that
	// Open Services is a dead end where Services will correctly show green.
	no_download_client: {
		detail: 'Prowlarr had no download client enabled to hand the release to',
		nonAction:
			'that switch is in Prowlarr under Settings → Download Clients, and nothing here can flip it',
		action: 'none'
	},
	no_indexers: {
		detail: 'Prowlarr had no enabled indexer that could carry this release',
		nonAction:
			'which indexers are on is Prowlarr’s own setting, so this screen can report it and not change it',
		action: 'none'
	},
	// UsArr's own guard, tripped by a client asserting a stale instance. Nothing
	// about the user's setup is wrong, so there is nothing for them to put right.
	instance_mismatch: {
		detail: 'the grab named a different service instance than the listing came from',
		nonAction:
			'UsArr held it back rather than send it to the wrong Prowlarr, so there is nothing here to put right',
		action: 'none'
	},
	internal: {
		detail: 'UsArr hit an error of its own before the release left',
		nonAction:
			'that one is UsArr’s fault rather than your setup’s, and the server log has the detail',
		action: 'none'
	},
	// ⚠️ THE ROW THIS RULE EXISTS FOR. grab_failed asserts only that the release
	// did not reach the download client; WHICH condition stopped it is exactly
	// what the code does not say. So the clause says that rather than picking one,
	// and the row offers nothing — every condition under it is unchanged by asking
	// again, and an action that grabs again here would be a button that cannot
	// work attached to the one code that cannot say why.
	grab_failed: {
		detail: 'UsArr could not say which of several things stopped it',
		nonAction:
			'nothing here puts it right: a credential Prowlarr rejected, an address UsArr refused, an ' +
			'indexer UsArr had stopped asking and a request Prowlarr would not take are all unmoved by asking again',
		action: 'none'
	}
};

/** The nameless variant of `not_found`: the candidate had been swept before UsArr
 * could read a title off it, so there is no query a fresh search could carry. */
const NOT_SENT_NAMELESS: NotSentCopy = {
	detail: 'the listing was gone when you pressed Grab — swept, or outside this account’s reach',
	nonAction: 'UsArr has no name for it either, so there is nothing to search for',
	action: 'none'
};

/** A code this build has never heard of. The store's vocabulary is open and so is
 * errorcodes.go's, so this is a state to render rather than an impossibility. */
const NOT_SENT_UNRECOGNISED: NotSentCopy = {
	detail: 'the reason UsArr recorded is one this screen does not know',
	nonAction:
		'the code on this row is newer than this screen, so it cannot say more or offer anything',
	action: 'none'
};

/**
 * What a Release cell holds when `release_title` is `""`.
 *
 * ⚠️ NOT AN EMPTY CELL, AND NEVER AN INVENTED NAME. The empty string is a FACT
 * on this arm rather than missing data: the candidate had been swept 25 minutes
 * after the search, so `release_candidate` — the only place the name ever lived
 * — no longer had it when the audit row was written. `NOTHING.empty`'s em dash
 * would render that as an unremarkable blank, which it is not; a guessed title
 * would be worse still, since the whole job of this column is letting a person
 * recognise which release a row is about.
 */
export const GRAB_MISSING_TITLE_NOTE =
	'no name recorded — the listing was already gone when you pressed Grab';

function notSentCopy(input: GrabOutcomeInput): NotSentCopy {
	const code = input.errorCode ?? '';
	// The nameless case first: it overrides whatever the code would have offered,
	// because an action needs something to aim at and this row has nothing.
	if (input.hasTitle === false) return NOT_SENT_NAMELESS;
	return NOT_SENT_BY_CODE[code] ?? NOT_SENT_UNRECOGNISED;
}

/**
 * Map an outcome onto the words §17.5 permits.
 *
 * THE UNCONFIRMED ROW SITS BESIDE THE CONFIRMED ONE, VISUALLY AND VERBALLY,
 * AND NEVER BESIDE THE NOT-SENT ONE. The first two both read "sent"; the
 * ambiguous one adds that Prowlarr reported a problem after accepting the
 * release and that UsArr cannot tell whether the download is affected. Pairing
 * them as opposites would lie in both directions — implying that a 200 confirms
 * a download and that a 500 means nothing happened, and neither is true. This
 * was a real incident, not an inference: the owner's book downloaded end to end
 * in Deluge while UsArr reported "Grab failed — HTTP 502".
 *
 * ⚠️ THE THIRD STATE IS THE ONE THAT MAY BE WORDED AS "IT DID NOT HAPPEN", AND
 * IT IS THE ONLY ONE. `not_sent` is written from `audit_log`, on the four
 * pre-dispatch paths where UsArr genuinely knows the release never left the
 * process — so the chip says `not sent` rather than "sent" anything, it takes
 * the err role rather than warn, and it is the only state that may carry an
 * action at all.
 *
 * ⚠️ AND IT STILL SAYS NOTHING §17.5 BANS. The ban list is unchanged and
 * absolute across all four states: no row anywhere says "succeeded",
 * "downloading", "done", "complete" or "failed", and none says "retry". "Not
 * sent" is this screen's own existing word for state 3 — `liveGrabCopy` chose it
 * for the live surface — and it is more precise than a verdict, because it names
 * where the request stopped rather than passing judgement on the release. §17.5
 * argues the case for `Search again` over `Retry` on exactly these grounds: a
 * guard with exceptions is one people argue with rather than obey, and the cost
 * of keeping it absolute is one word.
 *
 * An unrecognised state still reads "sent", because the store's vocabulary is
 * open by design — migration 0003 ships no CHECK constraint — and the one thing
 * a provenance row always means is that the request was dispatched. A NEW value
 * must therefore never be promoted into `not_sent` by accident: the switch is
 * full-string and exhaustive, and everything it does not name lands on the
 * handed-over reading, which is the safe direction for a value nobody has
 * defined yet.
 */
export function grabOutcome(
	outcome: string | undefined,
	input: GrabOutcomeInput = {}
): GrabOutcomeCopy {
	switch (outcome) {
		case OUTCOME_SENT:
			// No detail: see GrabOutcomeCopy.detail. The label is the whole fact,
			// and the block's own note above the table carries the rest once.
			return {
				outcome: OUTCOME_SENT,
				label: 'sent',
				tone: 'neutral',
				action: 'none',
				offersRetry: false
			};
		case OUTCOME_SENT_UNKNOWN:
			return {
				outcome: OUTCOME_SENT_UNKNOWN,
				label: 'sent, outcome unknown',
				tone: 'warn',
				detail: 'Prowlarr reported a problem after accepting it — check your download client',
				// No action, permanently. The only button that would fit is one that
				// sends the release again, which is what produces two copies of a
				// 68 GB release; the truth lives in the download client, and the
				// clause above points there in words rather than in a control.
				action: 'none',
				offersRetry: false
			};
		case OUTCOME_NOT_SENT: {
			const copy = notSentCopy(input);
			return {
				outcome: OUTCOME_NOT_SENT,
				// One chip for every code, deliberately. `.chip` is `white-space:
				// nowrap`, so a per-code label would overflow the cell it sits in —
				// and the code's own clause is directly under it, which is where §9.1
				// puts the fact that varies. The chip carries the STATE.
				label: 'not sent',
				tone: 'err',
				detail: copy.detail,
				nonAction: copy.nonAction === '' ? undefined : copy.nonAction,
				action: copy.action,
				offersRetry: false
			};
		}
		default:
			return {
				outcome: outcome && outcome.length > 0 ? outcome : OUTCOME_UNRECOGNISED,
				label: 'sent, state not recognised',
				tone: 'warn',
				detail: 'this row is newer than this screen',
				action: 'none',
				offersRetry: false
			};
	}
}

/** Every error code this build has a clause for, so a test can hold the whole
 * table against the ban list instead of a sample of it. */
export const NOT_SENT_CODES: readonly string[] = Object.keys(NOT_SENT_BY_CODE);

/**
 * The line above the block that says where UsArr's knowledge stops (§17.5).
 * It is above rather than below because it governs how every row is read.
 */
export const KNOWLEDGE_STOPS_NOTE =
	'UsArr stops watching a grab the moment Prowlarr accepts it. This is what UsArr handed over and ' +
	'what Prowlarr said at the time — not what your download client has managed since.';

/**
 * The line that says which grabs are missing from the block (§17.5), and it now
 * has TWO facts to carry at once rather than one.
 *
 * ⚠️ IT USED TO SAY THE NOT-SENT ARM WAS UNBUILT, AND THAT IS NO LONGER TRUE.
 * `GET /api/v1/grabs/recent` unions `provenance` with the `audit_log` rows
 * carrying `action='release.grab'` and `result='fail'`, so a rejected API key, a
 * refused address, a paused indexer and a release Prowlarr would not take are
 * all listed now, marked `not_sent`.
 *
 * ⚠️ AND THE OBVIOUS REPLACEMENT — "every grab is listed" — WOULD BE FALSE IN
 * THE OTHER DIRECTION, which is the whole reason this line is still here.
 * handleGrab has seven pre-dispatch early returns and only FOUR write an audit
 * row (internal/httpapi/grab.go). The three that do not are deliberate rather
 * than pending: no session (no actor, so the scoped read could never return the
 * row, and it is the one branch reachable without credentials — an append-only
 * table anyone who can reach the port could grow), a malformed path id, and an
 * undecodable body. None of the three ever named a release, so none of them
 * describes a grab, and a row for one would render as a failed grab of nothing.
 *
 * So the sentence has to be true in both directions at once: they are listed,
 * and the list is not every way a grab can go wrong. That is the whole of what
 * it says — the reasoning above is why, and the reasoning is not the user's
 * problem.
 */
export const NOT_SENT_NOTE =
	'Grabs that never got sent are listed here too, marked as such — but not all of them. A request ' +
	'UsArr turned away before it could name a release, from a signed-out tab or a malformed link, is ' +
	'a routing fault rather than a grab, so it goes to the log and not to this table.';

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
 * ⚠️ THE LIST DID NOT GROW AN EXCEPTION WHEN THE NOT-SENT STATE ARRIVED, AND
 * THAT WAS A DECISION RATHER THAN AN OVERSIGHT. §17.5 permits state 3 to be
 * worded as "it did not happen" — UsArr genuinely knows the release never left
 * the process — so "failed" would have been defensible on that arm alone. It is
 * not used, for the reason §17.5 gives for preferring `Search again` to `Retry`
 * on the merits-neutral case: a guard over words cannot see context, so the
 * choice is between a guard that is absolute and a guard that has exceptions,
 * and a guard with exceptions is one people argue with rather than obey. A
 * screen that bans a word in one state and uses it in the next teaches the next
 * author that the ban is soft. `not sent` is this screen's own word for state 3
 * already — `liveGrabCopy` chose it — and it is the more precise one: it names
 * where the request stopped instead of passing a verdict on the release.
 *
 * The list is exported so the test can hold every string in this module against
 * it, on every state including the not-sent one. That is the guard: a future
 * edit reintroducing "succeeded" fails a test rather than passing review.
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
 * the three densities, i.e. 1.67x the one-line default. That is this row's
 * shape, so those are the numbers rather than a fresh guess. `auto` in front of
 * the length still means the browser replaces the estimate with the row's own
 * size once it has seen one.
 */
export const RELEASE_ROW_INTRINSIC: Record<string, number> = {
	compact: 45,
	standard: 49,
	relaxed: 53
};

/* ── 10. The empty state, scoped by search type ───────────────────────────── */

/**
 * ⚠️ THREE EMPTY STATES, NOT ONE, AND THE MIDDLE ONE IS SCOPED BY MEDIA TYPE.
 *
 * DESIGN-DIRECTION §9.6 separates *you have not searched yet* from *your search
 * returned nothing*, and §17.5 adds the third that this screen specifically
 * needs: Prowlarr answers 200 with `[]` for "nothing matched" and for "every
 * indexer failed" alike, so one message over both tells a user their query is
 * wrong at the exact moment their indexers are down.
 *
 * THE TYPE SCOPING IS SW-08, and it is a claim about the indexer ecosystem
 * rather than about the query. 403 of Prowlarr's 543 indexer definitions are
 * private and the trackers where music and audiobooks actually live are
 * invite-only, so a stock Prowlarr searches almost none of the places those two
 * live: "nothing matched" there points the user at a query that was fine. For a
 * film or an episode the public indexers do carry the content, so an empty
 * answer from indexers that ANSWERED is more likely the query, and the useful
 * advice is the opposite one.
 *
 * ⚠️ WHAT IS DELIBERATELY NOT DIAGNOSED. `Basic` gets the mechanical fact — the
 * query is sent unchanged — and no cause at all, because the type is exactly
 * what tells the two causes apart and Basic withholds it. Guessing there would
 * be inventing a cause off a control the user set to "do not narrow this".
 *
 * ⚠️ AND `answered` OUTRANKS THE TYPE, ALWAYS. Neither sentence is true of a
 * fan-out nobody answered — the query was never tested and coverage was never
 * exercised — so `answered === 0` takes its own state and the type is not
 * consulted.
 */
export interface SearchEmptyInput {
	/** The query that was actually run. Empty means nothing has been searched. */
	submitted: string;
	/** The selected search type's id, one of `SEARCH_TYPES`. */
	typeId: string;
	/** Indexers that answered. A failed leg is not a response. */
	answered: number;
	/** Indexers in the fan-out, where the server has said. */
	total?: number;
}

/** §9.6's shape: a heading and one sentence naming why. No container, no
 * illustration, no centred block — `List.svelte` renders both in flow. */
export interface EmptyStateCopy {
	title: string;
	text: string;
}

/** "9 of 9 indexers answered", or "9 indexers answered" before the closing
 * report has named a denominator. Never invents a total. */
function answeredClause(answered: number, total?: number): string {
	if (total === undefined || total < 0)
		return `${plural(answered, 'indexer', 'indexers')} answered`;
	return `${answered} of ${total} ${total === 1 ? 'indexer' : 'indexers'} answered`;
}

/**
 * The words for the two types whose empty answer is usually COVERAGE, keyed by
 * type id. Separate constants rather than one templated sentence because the
 * nouns differ and a sentence assembled from a noun slot reads like one.
 */
const THIN_COVERAGE_EMPTY: Record<string, { title: string; because: string }> = {
	music: {
		title: 'No music releases matched',
		because:
			'The trackers that carry music are invite-only, so a stock Prowlarr searches almost none of ' +
			'the places music lives: thin coverage is a likelier explanation than your query, and adding ' +
			'a private indexer you already have an account on is what changes it.'
	},
	book: {
		title: 'No book releases matched',
		because:
			'The trackers that carry ebooks and audiobooks are invite-only, so a stock Prowlarr searches ' +
			'almost none of the places they live: thin coverage is a likelier explanation than your ' +
			'query, and adding a private indexer you already have an account on is what changes it.'
	}
};

/** The two types where the public indexers genuinely do carry the content, so an
 * answer of nothing from indexers that answered points at the query instead. */
const QUERY_LIKELY_EMPTY: Record<string, string> = {
	movie: 'films',
	tv: 'episodes and series'
};

export const EMPTY_IDLE_TITLE = 'Nothing searched yet';
export const EMPTY_IDLE_TEXT =
	'Free-text search across the indexers configured in Prowlarr. Results stream in per indexer as ' +
	'they answer, and Grab posts the release back to Prowlarr.';

export function searchEmptyState(input: SearchEmptyInput): EmptyStateCopy {
	const { submitted, typeId, answered, total } = input;
	if (submitted === '') return { title: EMPTY_IDLE_TITLE, text: EMPTY_IDLE_TEXT };

	const clause = answeredClause(answered, total);

	// §9.6's third state. Nothing below this line is true of it: the query was
	// never put to an indexer, so neither "your query matched nothing" nor "your
	// coverage is thin" is a statement UsArr is entitled to make.
	if (answered === 0) {
		return {
			title: 'No indexer answered',
			text:
				`${clause}, so this list is empty because nobody reported rather than because nothing ` +
				'matched, and your query has not actually been tested. What went wrong is stated once above.'
		};
	}

	const thin = THIN_COVERAGE_EMPTY[typeId];
	if (thin) {
		return { title: thin.title, text: `${clause} and none had a match. ${thin.because}` };
	}

	const carried = QUERY_LIKELY_EMPTY[typeId];
	if (carried) {
		return {
			title: 'No releases matched',
			text:
				`${clause} and none had a match for “${submitted}”. Public indexers do carry ${carried}, ` +
				'so where they answer and return nothing the query is the likelier explanation: UsArr sends ' +
				'what you type unchanged, and a title on its own matches more than a title with a year, an ' +
				'edition or a release group attached.'
		};
	}

	// Basic, and any id this build does not know. No cause is named, because the
	// type is the thing that would have named it.
	return {
		title: 'No releases matched',
		text:
			`${clause} and none had a match for “${submitted}”. An indexer matches against the release ` +
			'name it holds and UsArr sends your query unchanged, so a shorter query matches more than a ' +
			'full release name does.'
	};
}

/* ── 11. The correlated failure ───────────────────────────────────────────── */

/**
 * ⚠️ N COPIES OF ONE SENTENCE IS NOT A DIAGNOSIS, AND §17.5 SAYS SO: *naming the
 * non-action beats offering a fake one*.
 *
 * When every leg of a fan-out comes back the same way, the screen was printing
 * the per-indexer banner N times — nine titles, nine two-sentence explanations,
 * nine identical reasons — and the one fact worth having, that the nine are one
 * condition rather than nine, was the fact nowhere on screen.
 *
 * ⚠️ WHAT THE CLASS IS: THE SERVER'S OWN `status`, NOT A GUESS OFF ITS PROSE.
 * `internal/releases/search.go` sets exactly one of `ok`, `failed`, `timed_out`,
 * `blocked`, `breaker_open`, `disabled`, `unsupported`, `not_found` per leg, and
 * every one of those means something different about who is at fault and what,
 * if anything, can be done. The correlation being detected is therefore simply:
 * every leg carries the SAME status and none of them answered. That is read off
 * the wire; nothing here parses an error message to decide what happened.
 *
 * ⚠️ WHAT IS REFUSED. The one thing that is NOT asserted anywhere below is a
 * cause outside what UsArr observed. UsArr talks to Prowlarr and Prowlarr talks
 * to the indexers; UsArr sees the near half of that and infers nothing about the
 * far half. So the strong case reads "the same error came back from every
 * indexer, which is one fault rather than N, and it sits on the path they share"
 * — which is what the data supports — and never "Prowlarr cannot resolve DNS",
 * which is one of several conditions that produce exactly this evidence.
 * §17.5's example names a host-level resolution failure inside Prowlarr's
 * container; it is an EXAMPLE of the class, and printing it as the verdict would
 * be the invented status this repo's rules exist to stop.
 */

/**
 * The subset of `IndexerOutcome` ($lib/api) this reads. Declared structurally
 * rather than imported so this module stays free of the fetch layer, exactly as
 * the rest of the file is: `vitest.config.ts` is `environment: 'node'`.
 */
export interface IndexerLeg {
	name: string;
	/** The server's `status` field, verbatim. */
	status: string;
	answered: boolean;
	/** The server's human-readable, credential-free `reason`. */
	reason?: string;
	blockedUntil?: string;
}

/** One class per server status that a whole fan-out can land in together. */
export type CorrelatedKind =
	'shared_fault' | 'timeout' | 'breaker' | 'blocked' | 'disabled' | 'unsupported' | 'unknown_ids';

/**
 * The one action the screen may offer for a class, and `none` is a first-class
 * value rather than an absence: §17.5's rule is that where the correct action is
 * not something UsArr can offer, naming the non-action beats offering a fake one.
 *
 * `retry` is not in the union and never will be. The screen's word for asking
 * again is `search-again`, which is a fresh fan-out rather than a re-send of an
 * irreversible grab.
 */
export type CorrelatedAction = 'none' | 'search-again' | 'clear-scope';

export interface CorrelatedDiagnosis {
	kind: CorrelatedKind;
	/** How many indexers this covers. Always the whole fan-out, by construction. */
	indexers: number;
	title: string;
	/** What was observed, and the most it supports. Hedged where it is a pattern
	 * rather than a cause. */
	text: string;
	/** The sentence that names what this screen cannot do. Empty ONLY where the
	 * offered action genuinely fixes the condition. */
	nonAction: string;
	/** The upstream's own words, once, where every leg gave the same ones. Empty
	 * where they differ, and the caller then lists them per indexer instead —
	 * a blocked indexer's reason carries its own unblock time. */
	verbatim: string;
	action: CorrelatedAction;
	tone: 'warn' | 'err';
}

const CORRELATED_COPY: Record<
	CorrelatedKind,
	(n: number) => Omit<CorrelatedDiagnosis, 'kind' | 'indexers' | 'verbatim'>
> = {
	shared_fault: (n) => ({
		title: `All ${n} indexers came back with the same error`,
		text:
			`${n} separate indexers do not usually produce one identical error at the same moment, so ` +
			'this is one fault rather than ' +
			`${n}. Whatever it is sits on the path they share, which is UsArr to Prowlarr and Prowlarr ` +
			'outward, rather than at the indexers themselves.',
		nonAction:
			'UsArr sees its own half of that path and no further, so it cannot tell you which part of it ' +
			'is at fault. The sentence below is Prowlarr’s own and is the whole of what UsArr was told.',
		action: 'search-again',
		tone: 'err'
	}),
	timeout: (n) => ({
		title: `None of the ${n} indexers answered inside the deadline`,
		text:
			`All ${n} legs hit the same per-indexer deadline with nothing to show, which is one condition ` +
			`rather than ${n}. Indexers timing out together usually means the requests are not getting ` +
			'through: a slow or unreachable Prowlarr looks exactly like this from here.',
		nonAction:
			'Which end is slow is not visible from this screen. UsArr measures how long Prowlarr took to ' +
			'answer and nothing beyond it.',
		action: 'search-again',
		tone: 'err'
	}),
	breaker: (n) => ({
		title: `UsArr has stopped asking all ${n} indexers`,
		text:
			'These are UsArr’s own circuit breakers rather than anything Prowlarr said. Each opened after ' +
			'that indexer kept failing, and all of them being open at once usually means those failures ' +
			'were shared rather than particular to one indexer.',
		nonAction:
			'There is nothing to press here: each breaker closes by itself at the time its line below ' +
			'gives, and asking again before then asks nobody.',
		action: 'none',
		tone: 'err'
	}),
	blocked: (n) => ({
		title: `Prowlarr reports all ${n} indexers as unavailable`,
		text:
			'This is Prowlarr’s own indexer status rather than UsArr’s: it has marked every one of them ' +
			'as failing and will not query them until the times below. UsArr forwards that verdict ' +
			'instead of forming its own.',
		nonAction:
			'This screen can read that state and cannot change it. It clears in Prowlarr, either on its ' +
			'own or from Prowlarr’s own indexer page.',
		action: 'none',
		tone: 'err'
	}),
	disabled: (n) => ({
		title: `All ${n} indexers are switched off in Prowlarr`,
		text:
			'Every indexer in the fan-out is disabled at Prowlarr’s end, so none of them was asked. ' +
			'Nothing on the path is broken and UsArr did not skip them; they are turned off.',
		nonAction:
			'The switch belongs to Prowlarr rather than to UsArr, so this screen can report it and ' +
			'cannot flip it.',
		action: 'none',
		tone: 'warn'
	}),
	unsupported: (n) => ({
		title: `All ${n} indexers declined this search`,
		text:
			'Each one was asked and each said it does not handle this search, so nothing is broken. ' +
			'Where every indexer says that, the mismatch is in what was asked for rather than in the ' +
			'indexers: a search type, a mode or a category none of yours offers.',
		nonAction:
			'Narrowing the search will not help and neither will asking again. The reason each one gave ' +
			'is below, and it names what it will not do.',
		action: 'none',
		tone: 'warn'
	}),
	unknown_ids: (n) => ({
		title: `This Prowlarr has none of the ${n} indexers you named`,
		text:
			'The indexer selection sent with this search names ids this Prowlarr does not have. That is a ' +
			'fact about the request rather than about any indexer, and it is what a shared link or a ' +
			'remembered selection from a different Prowlarr looks like.',
		// The one class with an action that actually resolves it, so it names no
		// non-action: saying "there is nothing you can do" beside a button that
		// works would be its own kind of dishonesty.
		nonAction: '',
		action: 'clear-scope',
		tone: 'warn'
	})
};

/** Server statuses that map straight onto a class. `failed` is absent on
 * purpose: it is the unclassified remainder and needs the extra evidence below
 * before it may be called one fault. */
const STATUS_KIND: Record<string, CorrelatedKind> = {
	timed_out: 'timeout',
	breaker_open: 'breaker',
	blocked: 'blocked',
	disabled: 'disabled',
	unsupported: 'unsupported',
	not_found: 'unknown_ids'
};

/**
 * The correlated diagnosis, or `undefined` where the evidence does not support
 * one and the caller should keep rendering per-indexer banners.
 *
 * FOUR CONDITIONS, AND EVERY ONE OF THEM IS A REFUSAL TO OVERSTATE:
 *
 *  1 **At least two legs.** One indexer failing is that indexer, and there is no
 *    repetition to collapse — a "correlation" over a single sample is a word
 *    dressed up as a finding.
 *  2 **Every leg is the whole fan-out.** `total` is checked where the server has
 *    named one, so a partially-reported live search cannot fire this off the
 *    first three legs that happen to have failed together.
 *  3 **Nobody answered.** One indexer answering makes this a partial result with
 *    a gap, which is what the per-indexer banners already say correctly.
 *  4 **One status across every leg**, and for the `failed` remainder ALSO one
 *    identical reason. Mixed statuses are several conditions and get the several
 *    banners they deserve; `failed` with differing errors is N different errors,
 *    which is precisely not the thing being detected.
 */
export function correlatedFailure(
	legs: readonly IndexerLeg[],
	total?: number
): CorrelatedDiagnosis | undefined {
	if (legs.length < 2) return undefined;
	if (total !== undefined && total >= 0 && legs.length !== total) return undefined;
	if (legs.some((l) => l.answered)) return undefined;

	const status = legs[0].status;
	if (legs.some((l) => l.status !== status)) return undefined;

	const reasons = new Set(legs.map((l) => (l.reason ?? '').trim()));
	const shared = reasons.size === 1 ? [...reasons][0] : '';

	let kind = STATUS_KIND[status];
	if (kind === undefined) {
		// The `failed` remainder, and the only place a reason is consulted at
		// all — not to classify what went wrong, but to establish that there is
		// ONE thing rather than N. An empty shared reason is no evidence either.
		if (status !== 'failed' || shared === '') return undefined;
		kind = 'shared_fault';
	}

	return { kind, indexers: legs.length, verbatim: shared, ...CORRELATED_COPY[kind](legs.length) };
}

/**
 * The per-leg reasons as ONE pre-wrapped block, for the classes whose legs agree
 * on the status and not on the words — a blocked or breaker-open set, where each
 * line carries its own time and dropping the lines would drop real data.
 *
 * One `.verbatim` box of N lines rather than N boxes: the repetition §17.5
 * objects to is the surrounding explanation, not the upstream's own sentences,
 * and each of those still appears exactly once. `.verbatim` is `pre-wrap`, so
 * the newline is the whole layout.
 */
export function legReasons(legs: readonly IndexerLeg[]): string {
	return legs.map((l) => `${l.name}: ${l.reason ?? l.status}`).join('\n');
}
