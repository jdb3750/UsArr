/**
 * The Requests screen's pure half.
 *
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so
 * `+page.svelte` cannot be imported here at all — the same limit `list.test.ts`
 * documents. The split is the point: the logic that can be wrong in a way a
 * reader would not notice lives in `requests.ts` and is asserted below, and the
 * component is left with markup.
 *
 * The outcome-vocabulary block is the one that matters most. §17.5 bans
 * specific words on this screen because a false "failed" invites a user to grab
 * the same 68 GB release twice and a grab is irreversible from UsArr's side, so
 * the ban is checked against the strings that actually ship rather than trusted
 * to review.
 */
import { describe, expect, it } from 'vitest';
// The Requests template AS TEXT, not as a component. `?raw` is Vite's own
// mechanism and it types as `string`, which is what lets the copy guard at the
// bottom of this file hold the screen's inline strings against §17.5's ban list
// in a `environment: 'node'` run with no Svelte plugin. `node:fs` would have
// been the obvious way and is not available: this workspace ships no
// `@types/node`, and adding one to read one file is a dependency for nothing.
import SCREEN_SOURCE from '../routes/requests/+page.svelte?raw';
import {
	DEFAULT_SEARCH_TYPE,
	FORBIDDEN_OUTCOME_WORDS,
	KNOWLEDGE_STOPS_NOTE,
	NOT_SENT_NOTE,
	OUTCOME_SENT,
	OUTCOME_SENT_UNKNOWN,
	RECENT_GRAB_ROW_INTRINSIC,
	SEARCH_TYPES,
	SEARCH_TYPE_NOTE,
	THIN_COVERAGE_NOTE,
	fanoutSummary,
	formatAbsolute,
	formatDate,
	formatRelative,
	formatWhen,
	grabOutcome,
	isFreeTextOnly,
	searchTypeParam
} from './requests';

describe('search types', () => {
	it('maps every label to a value internal/httpapi/search.go accepts', () => {
		// setSearchType switches over exactly these five and answers 400 to
		// anything else. It has to be an explicit list on both sides: Prowlarr
		// does NOT error on an unrecognised type, it silently degrades to a
		// basic search, which reads as "the filter did nothing" and is very hard
		// to diagnose from the outside.
		expect(SEARCH_TYPES.map((t) => t.param)).toEqual([
			'search',
			'movie',
			'tvsearch',
			'music',
			'book'
		]);
	});

	it('is the five Prowlarr offers, in Prowlarr’s own words', () => {
		expect(SEARCH_TYPES.map((t) => t.label)).toEqual(['Basic', 'Movie', 'TV', 'Music', 'Book']);
	});

	it('sends tvsearch for the option labelled TV', () => {
		// The one mapping where the label and the wire value differ, which is
		// exactly the one a hand-written template gets wrong.
		expect(searchTypeParam('tv')).toBe('tvsearch');
	});

	it('falls back to a basic search rather than sending an unknown type', () => {
		// A 400 for a value the UI produced is worse than a broader search: the
		// user did nothing wrong and has nothing to fix.
		expect(searchTypeParam('comics')).toBe('search');
		expect(searchTypeParam('')).toBe('search');
	});

	it('defaults to a type that exists', () => {
		expect(SEARCH_TYPES.some((t) => t.id === DEFAULT_SEARCH_TYPE)).toBe(true);
	});

	it('marks Book and Music as free text only, and nothing else', () => {
		// Prowlarr's SearchResource carries query, type, indexerIds, categories,
		// limit and offset. There is no author field and no artist field, so
		// these two look like structured modes and are not.
		expect(SEARCH_TYPES.filter((t) => t.freeTextOnly).map((t) => t.id)).toEqual(['music', 'book']);
		expect(isFreeTextOnly('book')).toBe(true);
		expect(isFreeTextOnly('music')).toBe(true);
		expect(isFreeTextOnly('movie')).toBe(false);
	});

	it('says so in the toolbar’s standing note, naming both types', () => {
		// §8.5 requires the UI to say it AT THE CONTROL. The only place it used
		// to be said was an empty state the user reaches after already failing.
		expect(SEARCH_TYPE_NOTE).toContain('free text');
		expect(SEARCH_TYPE_NOTE).toContain('Book cannot search by author');
		expect(SEARCH_TYPE_NOTE).toContain('Music cannot search by artist');
	});
});

describe('fanoutSummary', () => {
	it('renders §17.5’s sentence from real counts', () => {
		expect(
			fanoutSummary({ answered: 9, total: 9, releases: 112, deduped: 10, finished: true })
		).toBe('9 of 9 indexers responded · 112 releases, 10 after de-duplication');
	});

	it('says "shown" only once a results table is on screen', () => {
		// The word is a claim about the SCREEN. While the release table is still
		// on /search, nothing is shown and the sentence would be false; flipping
		// `rendered` with the table is the seam.
		expect(
			fanoutSummary({
				answered: 9,
				total: 9,
				releases: 112,
				deduped: 10,
				finished: true,
				rendered: true
			})
		).toBe('9 of 9 indexers responded · 112 releases, 10 shown after de-duplication');
	});

	it('marks a running fan-out as partial rather than final', () => {
		expect(
			fanoutSummary({ answered: 4, total: 9, releases: 47, deduped: 5, finished: false })
		).toBe('4 of 9 indexers responded · 47 releases so far, 5 after de-duplication');
	});

	it('counts only the indexers that answered, so a refusal is visible as a gap', () => {
		// The failing indexer's own banner names it; this figure is what stops
		// the list reading as whole.
		expect(fanoutSummary({ answered: 8, total: 9, releases: 96, deduped: 9, finished: true })).toBe(
			'8 of 9 indexers responded · 96 releases, 9 after de-duplication'
		);
	});

	it('drops the de-duplication clause when nothing was de-duplicated', () => {
		// "12 releases, 12 after de-duplication" is true and says nothing.
		expect(
			fanoutSummary({ answered: 3, total: 3, releases: 12, deduped: 12, finished: true })
		).toBe('3 of 3 indexers responded · 12 releases');
	});

	it('does not invent a total it has not been told', () => {
		// Before the closing report there is no authoritative denominator, and
		// "4 of 4" would claim the fan-out is complete.
		expect(fanoutSummary({ answered: 4, releases: 20, deduped: 20, finished: false })).toBe(
			'4 indexers responded · 20 releases so far'
		);
	});

	it('is singular where the number is one', () => {
		expect(fanoutSummary({ answered: 1, total: 1, releases: 1, deduped: 1, finished: true })).toBe(
			'1 of 1 indexer responded · 1 release'
		);
	});

	it('says zero rather than nothing when a search returned nothing', () => {
		// Prowlarr answers 200 with [] for "nothing matched" and for "every
		// indexer failed" alike, so the count is not the whole story — but it
		// must still be stated rather than left blank.
		expect(fanoutSummary({ answered: 9, total: 9, releases: 0, deduped: 0, finished: true })).toBe(
			'9 of 9 indexers responded · 0 releases'
		);
	});
});

describe('grabOutcome', () => {
	it('reads a confirmed grab as sent, and never as a confirmation of anything else', () => {
		const copy = grabOutcome(OUTCOME_SENT);
		expect(copy.label).toBe('sent');
		expect(copy.tone).toBe('neutral');
	});

	it('puts the ambiguous row beside the confirmed one, not beside a failure', () => {
		// The boundary that matters is handed-over versus not-handed-over. Both
		// of these were handed over; they differ only in whether an error came
		// back afterwards. A row that treats them as opposites lies in both
		// directions — implying a 200 confirms a download and a 500 means
		// nothing happened.
		const sent = grabOutcome(OUTCOME_SENT);
		const unknown = grabOutcome(OUTCOME_SENT_UNKNOWN);
		expect(sent.label.startsWith('sent')).toBe(true);
		expect(unknown.label.startsWith('sent')).toBe(true);
		expect(unknown.label).toBe('sent, outcome unknown');
	});

	it('points the ambiguous row at the download client, where the truth is', () => {
		// Not at Test connection: this is not a connectivity failure, the test
		// will pass, and it sends the user to check something that is working.
		expect(grabOutcome(OUTCOME_SENT_UNKNOWN).detail).toContain('download client');
	});

	it('gives the confirmed row no sub-clause, because the chip is the whole fact', () => {
		// §9.1: a value identical for every row of a group is not data — state it
		// once in the group header and drop it from the rows. "the client
		// accepted it" was the chip restated under every confirmed row, at a
		// second line each, and the fact it carried is above the table already
		// (KNOWLEDGE_STOPS_NOTE opens on the moment Prowlarr accepts a grab).
		expect(grabOutcome(OUTCOME_SENT).detail).toBeUndefined();
		expect(KNOWLEDGE_STOPS_NOTE).toContain('the moment Prowlarr accepts it');
	});

	it('keeps the clause on the two states where it is an instruction, not a restatement', () => {
		// The test is §9.1's: delete this clause — does the user lose a fact they
		// can act on? Yes for these two, no for `sent`. Both say something the
		// chip cannot: where to look, and why this screen cannot read the row.
		expect(grabOutcome(OUTCOME_SENT_UNKNOWN).detail).toBeTruthy();
		expect(grabOutcome('pending').detail).toBeTruthy();
	});

	it('never offers Retry, on any state', () => {
		// Retry means "do it again", and doing it again is exactly what produces
		// two copies of a 68 GB release.
		for (const value of [OUTCOME_SENT, OUTCOME_SENT_UNKNOWN, 'unknown', 'pending', '']) {
			expect(grabOutcome(value).offersRetry).toBe(false);
		}
	});

	it('still reads as sent for a state this screen does not recognise', () => {
		// migration 0003 ships NO CHECK constraint on purpose, so the vocabulary
		// is open and v0.2 may add to it. Every provenance row means the request
		// was dispatched, so "sent" stays true whatever the state says.
		const copy = grabOutcome('pending');
		expect(copy.label.startsWith('sent')).toBe(true);
		expect(copy.label).toBe('sent, state not recognised');
		expect(copy.outcome).toBe('pending');
	});

	it('does not promote a missing outcome to the confirmed wording', () => {
		expect(grabOutcome(undefined).label).toBe('sent, state not recognised');
		expect(grabOutcome(undefined).outcome).toBe('unknown');
	});

	it('distinguishes the two readable states without colour', () => {
		// §9.5: removing the colour must leave a status fully legible. Two chips
		// reading the same word in two colours would fail that.
		expect(grabOutcome(OUTCOME_SENT).label).not.toBe(grabOutcome(OUTCOME_SENT_UNKNOWN).label);
	});

	it('uses only the two chroma roles, and marks the unknown one rather than the fine one', () => {
		// §9.5: chroma marks what is wrong, not what is fine. The ordinary sent
		// row is the one with nothing to do about it, so it is neutral.
		expect(grabOutcome(OUTCOME_SENT).tone).toBe('neutral');
		expect(grabOutcome(OUTCOME_SENT_UNKNOWN).tone).toBe('warn');
	});
});

describe('the banned vocabulary', () => {
	// Every user-facing string this module ships, held against §17.5's ban list.
	// This is the guard: an edit that reintroduces "succeeded" or wires a
	// "failed" chip fails here rather than passing review.
	const strings = [
		SEARCH_TYPE_NOTE,
		THIN_COVERAGE_NOTE,
		KNOWLEDGE_STOPS_NOTE,
		NOT_SENT_NOTE,
		...[OUTCOME_SENT, OUTCOME_SENT_UNKNOWN, 'unknown'].flatMap((o) => {
			const copy = grabOutcome(o);
			return [copy.label, copy.detail ?? ''];
		})
	];

	it.each(FORBIDDEN_OUTCOME_WORDS)('never says “%s”', (word) => {
		for (const value of strings) {
			expect(value.toLowerCase()).not.toContain(word);
		}
	});

	it('never asserts that a grab did not happen', () => {
		// The one unknowable claim. Prowlarr adds the release to the download
		// client BEFORE the step that failed for the owner, and never rolls
		// back, so a 5xx can cover an operation that already partly succeeded.
		for (const value of strings) {
			expect(value.toLowerCase()).not.toContain('did not go through');
			expect(value.toLowerCase()).not.toContain('nothing was sent');
		}
	});

	it('says where UsArr’s knowledge stops, and which grabs are missing', () => {
		// §17.5 requires both lines. Without the second, a block that lists only
		// dispatched grabs reads as "every grab worked".
		expect(KNOWLEDGE_STOPS_NOTE).toContain('stops watching');
		expect(NOT_SENT_NOTE).toContain('never got sent');
	});
});

/* ── The same ban, over the copy that is NOT in this module ──────────────── */

/**
 * WHY THIS READS A FILE INSTEAD OF IMPORTING ONE.
 *
 * The ban above only ever held the strings `requests.ts` exports, and roughly
 * half the words on the Requests screen are not among them: banner titles,
 * `emptyText`, the block-3 placeholder and the column headers are written
 * inline in `+page.svelte`. §17.5's rule is about what reaches the user, not
 * about which file it was typed in, so a guard that stops at the module boundary
 * is a guard with a hole exactly where the next edit will be — nobody adds copy
 * to a constants module by accident, they add it to the template.
 *
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so the
 * component cannot be imported and compiled here. It can be read AS TEXT with
 * Vite's `?raw`, and text is what the ban is about. So: take the source, drop
 * `<script>`, `<style>` and HTML comments — none of which reaches a user — and
 * hold the remainder against the list. Attributes stay in, deliberately:
 * `emptyTitle`, `emptyText` and `loadingNote` are user-facing strings that live
 * in attributes.
 *
 * TWO REGIONS, TWO RULE SETS, AND THE DISTINCTION IS THE POINT.
 *
 * §17.5's ban is on what may be said about A GRAB, whose outcome UsArr stops
 * observing at handoff. It is not a ban on the words themselves: UsArr watches a
 * SEARCH from the first indexer to the closing report, so "The search did not
 * complete" is a true sentence about something UsArr actually observed, and a
 * guard that flags it would be teaching the next author to write vaguer copy
 * about a fact the app genuinely has. So the grab block — everything inside
 * `<section id="recent-grabs">`, where every string is about a grab — carries
 * the full list, and the rest of the screen carries the subset that is a claim
 * about a download no matter what sentence it appears in.
 */
const SCREEN_MARKUP = SCREEN_SOURCE.replace(/<script[\s\S]*?<\/script>/gi, ' ')
	.replace(/<style[\s\S]*?<\/style>/gi, ' ')
	.replace(/<!--[\s\S]*?-->/g, ' ');

/** Everything inside the Recent-grabs section, which is the region where every
 * string on screen is a statement about a grab. */
const GRAB_BLOCK_MARKUP = (() => {
	const open = SCREEN_MARKUP.indexOf('id="recent-grabs"');
	const close = SCREEN_MARKUP.indexOf('</section>', open);
	// A guard that silently matches nothing is indistinguishable from no guard,
	// so a moved marker fails here rather than quietly narrowing the check to an
	// empty string.
	if (open < 0 || close < 0) throw new Error('the Recent-grabs section markers moved');
	return SCREEN_MARKUP.slice(open, close);
})();

/** The rest of the screen: the toolbar, the fan-out line, every banner and the
 * block-3 placeholder. */
const NON_GRAB_MARKUP = SCREEN_MARKUP.replace(GRAB_BLOCK_MARKUP, ' ');

/**
 * The words that are a claim about a download wherever they appear, so they are
 * banned across the whole screen rather than only inside the grab block.
 *
 * `complete` and `failed` are deliberately NOT here. They are banned about a
 * grab and true about a search — `+page.svelte`'s "The search did not complete"
 * is the live case, and it reports a thing UsArr watched end to end.
 */
const NEVER_ANYWHERE_ON_THIS_SCREEN = FORBIDDEN_OUTCOME_WORDS.filter(
	(word) => word !== 'complete' && word !== 'failed'
);

describe('the banned vocabulary, in the screen’s own markup', () => {
	it('is reading the file it thinks it is reading', () => {
		// The failure mode of a text guard is matching nothing at all. These are
		// strings only the Requests template carries.
		expect(SCREEN_MARKUP).toContain('Recent grabs');
		expect(GRAB_BLOCK_MARKUP).toContain('No grabs recorded yet');
		expect(NON_GRAB_MARKUP).toContain('There is no indexer service to search');
		expect(NON_GRAB_MARKUP).not.toContain('No grabs recorded yet');
	});

	it.each(FORBIDDEN_OUTCOME_WORDS)('never says “%s” about a grab', (word) => {
		expect(GRAB_BLOCK_MARKUP.toLowerCase()).not.toContain(word);
	});

	it.each(NEVER_ANYWHERE_ON_THIS_SCREEN)('never says “%s” anywhere on the screen', (word) => {
		expect(NON_GRAB_MARKUP.toLowerCase()).not.toContain(word);
	});

	it('leaves search copy free to report what UsArr actually watched', () => {
		// The counter-case, pinned so a later tightening of the guard has to face
		// it: this sentence is correct and must keep passing. UsArr observes a
		// search from the first indexer to the closing report, so "did not
		// complete" is a fact rather than the unknowable claim §17.5 bans.
		expect(NON_GRAB_MARKUP).toContain('The search did not complete');
	});

	it('never asserts in the markup that a grab did not happen', () => {
		expect(GRAB_BLOCK_MARKUP.toLowerCase()).not.toContain('did not go through');
		expect(GRAB_BLOCK_MARKUP.toLowerCase()).not.toContain('nothing was sent');
	});
});

describe('timestamps', () => {
	// Built from local components on purpose: the formatters read local-time
	// getters, so constructing both sides this way makes the assertions
	// independent of the container's timezone.
	const now = new Date(2026, 7, 16, 14, 8, 30);

	it('renders one date format — no leading zero, always with the year', () => {
		expect(formatDate(new Date(2026, 7, 8))).toBe('8 Aug 2026');
	});

	it('is a bare clock inside 24 hours', () => {
		expect(formatAbsolute(new Date(2026, 7, 16, 14, 7), now)).toBe('14:07');
	});

	it('carries a date past 24 hours, which is when the clock alone stops identifying it', () => {
		expect(formatAbsolute(new Date(2026, 7, 15, 12, 0), now)).toBe('12:00 on 15 Aug 2026');
	});

	it('measures those 24 hours in elapsed time, not in calendar days', () => {
		// 22:12 "yesterday" is 15 hours ago, and §9.1's threshold is the point
		// at which a bare clock stops identifying a moment — which is elapsed
		// time. The relative form travels beside it either way, so a bare 22:12
		// is never ambiguous on screen.
		expect(formatAbsolute(new Date(2026, 7, 15, 22, 12), now)).toBe('22:12');
		expect(formatRelative(new Date(2026, 7, 15, 22, 12), now)).toBe('15 hours ago');
	});

	it('pads the clock so the column stays aligned', () => {
		expect(formatAbsolute(new Date(2026, 7, 16, 9, 5), now)).toBe('09:05');
	});

	it('reads a fresh grab as “just now” rather than “0 minutes ago”', () => {
		expect(formatRelative(new Date(2026, 7, 16, 14, 8, 10), now)).toBe('just now');
	});

	it('tolerates a small clock skew instead of saying “in the future”', () => {
		// A browser and a server disagreeing by seconds is ordinary, and a row
		// reading "-1 minutes ago" would look like a bug in the data.
		expect(formatRelative(new Date(2026, 7, 16, 14, 8, 45), now)).toBe('just now');
	});

	it('counts minutes, then hours, then days', () => {
		expect(formatRelative(new Date(2026, 7, 16, 14, 7), now)).toBe('1 minute ago');
		expect(formatRelative(new Date(2026, 7, 16, 13, 41), now)).toBe('27 minutes ago');
		expect(formatRelative(new Date(2026, 7, 16, 11, 8), now)).toBe('3 hours ago');
		expect(formatRelative(new Date(2026, 7, 12, 14, 8), now)).toBe('4 days ago');
	});

	it('says yesterday rather than “1 days ago”', () => {
		expect(formatRelative(new Date(2026, 7, 15, 12, 0), now)).toBe('yesterday');
	});

	it('coarsens past a month, because this block answers “an hour ago?” not “when exactly?”', () => {
		expect(formatRelative(new Date(2026, 4, 16, 14, 8), now)).toBe('3 months ago');
		expect(formatRelative(new Date(2024, 7, 16, 14, 8), now)).toBe('2 years ago');
	});

	it('carries both forms of one timestamp, which is §17.3’s rule', () => {
		const iso = new Date(2026, 7, 16, 14, 7).toISOString();
		expect(formatWhen(iso, now)).toEqual({ absolute: '14:07', relative: '1 minute ago' });
	});

	it('renders nothing rather than a guess when the timestamp is absent or unparseable', () => {
		// A wrong timestamp on an irreversible action is worse than a missing
		// one: this block is how a user answers "did I already grab this?".
		expect(formatWhen(undefined, now)).toEqual({ absolute: '', relative: '' });
		expect(formatWhen('not a date', now)).toEqual({ absolute: '', relative: '' });
	});
});

describe('RECENT_GRAB_ROW_INTRINSIC', () => {
	it('is app.css’s --row-h-two-line at each density', () => {
		// 🔍 These are the stylesheet's declared two-line row heights rather than
		// a measurement — this row is a 13/18 title over a 12/16 sub-line, which
		// is exactly what that token is declared for. `contain-intrinsic-size`
		// takes `auto` in front of the length, so the browser replaces the
		// estimate with the row's own size once it has seen it and the value
		// only has to be right for rows that have never been on screen. Pinned
		// here so a change to those tokens fails rather than drifting silently.
		expect(RECENT_GRAB_ROW_INTRINSIC).toEqual({ compact: 44, standard: 48, relaxed: 52 });
	});

	it('carries a value for every density the preferences can be set to', () => {
		for (const density of ['compact', 'standard', 'relaxed']) {
			expect(RECENT_GRAB_ROW_INTRINSIC[density]).toBeGreaterThan(0);
		}
	});
});
