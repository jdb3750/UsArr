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
	PRIORITY_NOTE,
	clearScopeLabel,
	indexerFacts,
	indexerPickerLegend,
	pickerScopeNote,
	scopeSummary,
	serviceFacts,
	unavailableReason
} from './indexercatalog';
import {
	DEFAULT_SEARCH_TYPE,
	EMPTY_IDLE_TITLE,
	FORBIDDEN_OUTCOME_WORDS,
	GRAB_MISSING_TITLE_NOTE,
	KNOWLEDGE_STOPS_NOTE,
	NOT_SENT_CODES,
	NOT_SENT_NOTE,
	OUTCOME_NOT_SENT,
	OUTCOME_SENT,
	OUTCOME_SENT_UNKNOWN,
	RECENT_GRAB_ROW_INTRINSIC,
	SEARCH_TYPES,
	SEARCH_TYPE_NOTE,
	THIN_COVERAGE_NOTE,
	correlatedFailure,
	fanoutSummary,
	formatAbsolute,
	formatDate,
	formatRelative,
	formatWhen,
	grabOutcome,
	isFreeTextOnly,
	legReasons,
	requestsSearchHref,
	searchEmptyState,
	searchTypeParam,
	type IndexerLeg
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

describe('grabOutcome — the not-sent state', () => {
	// §17.5's third state, which the endpoint could not express until the union
	// with audit_log landed. It is the ONE state where "nothing is running" is a
	// true claim, and the one that may carry an action.
	const notSent = (errorCode?: string, hasTitle = true) =>
		grabOutcome(OUTCOME_NOT_SENT, { errorCode, hasTitle });

	it('reads as not sent, and sits beside neither handed-over state', () => {
		// The boundary is handed-over versus not-handed-over. The first two are
		// the same epistemic state about the download; this is the other side, so
		// it must not read as a variety of "sent".
		const copy = notSent('expired');
		expect(copy.label).toBe('not sent');
		expect(copy.label.startsWith('sent')).toBe(false);
		expect(copy.tone).toBe('err');
		expect(copy.tone).not.toBe(grabOutcome(OUTCOME_SENT).tone);
		expect(copy.tone).not.toBe(grabOutcome(OUTCOME_SENT_UNKNOWN).tone);
	});

	it('is matched on the FULL string, never on a "sent" prefix', () => {
		// `not_sent` deliberately does not begin with `sent`, and the near miss
		// runs the other way: a membership test written in the obvious spirit —
		// `outcome.includes('sent')` — answers true for the one state where
		// nothing was handed over. Pinned so a later refactor to a prefix or a
		// substring match fails here.
		expect(OUTCOME_NOT_SENT.startsWith(OUTCOME_SENT)).toBe(false);
		expect(OUTCOME_NOT_SENT.includes(OUTCOME_SENT)).toBe(true);
		expect(notSent('expired').outcome).toBe(OUTCOME_NOT_SENT);
	});

	it('does not promote an unrecognised outcome into it', () => {
		// The safe direction for a value nobody has defined yet is the
		// handed-over reading: "sent" is true of every provenance row, and
		// asserting "not sent" is the one claim this screen may only make when
		// the server has made it first.
		for (const value of ['pending', 'not_sent_yet', 'sent_', '', undefined]) {
			expect(grabOutcome(value).label).not.toBe('not sent');
			expect(grabOutcome(value).tone).not.toBe('err');
		}
	});

	it('never offers Retry, and never an action that grabs again', () => {
		// The union has no member that re-sends a grab, by construction:
		// `search-again` starts a fresh fan-out and `open-services` navigates.
		for (const code of [...NOT_SENT_CODES, 'quantum', '']) {
			const copy = notSent(code);
			expect(copy.offersRetry).toBe(false);
			expect(['none', 'search-again', 'open-services']).toContain(copy.action);
		}
	});

	it('gives every code a clause, because the chip is identical on all of them', () => {
		// One chip for every code — `.chip` is `white-space: nowrap` and a
		// per-code label would overflow the cell — so the clause is the ONLY
		// thing separating one not-sent row from another. A code with no clause
		// would render as a bare "not sent" with nothing to act on.
		for (const code of NOT_SENT_CODES) {
			expect(notSent(code).detail).toBeTruthy();
		}
	});

	it('reads an expired listing as expired, and offers Search again', () => {
		// §17.5 by name: retrying the same opaque release id returns the same 4xx
		// for ever, so the one action that can work is a fresh search — and the
		// row must not read as a rejection, which says "the tracker refused you".
		const copy = notSent('expired');
		expect(copy.detail).toContain('expired');
		expect(copy.action).toBe('search-again');
		expect(`${copy.detail} ${copy.nonAction ?? ''}`.toLowerCase()).not.toContain('rejected');
	});

	it('offers Search again only where a fresh listing is what was missing', () => {
		// Those three are the codes that mean Prowlarr's 30-minute cache, or the
		// candidate row behind it, no longer holds the release. Every other code
		// describes something a new search would meet again unchanged.
		const researchable = NOT_SENT_CODES.filter((c) => notSent(c).action === 'search-again');
		expect(researchable.sort()).toEqual(
			['expired', 'no_longer_offered', 'not_found', 'search_failed'].sort()
		);
	});

	it('never offers an action on grab_failed, which cannot say what stopped it', () => {
		// The unclassified remainder: a rejected credential, a refused address, a
		// paused indexer, a request Prowlarr would not take, a corrupt blob. Not
		// one is moved by asking again, and the code cannot say which it was — so
		// the honest surface is the non-action, named.
		const copy = notSent('grab_failed');
		expect(copy.action).toBe('none');
		expect(copy.nonAction).toBeTruthy();
		expect(copy.nonAction).toContain('nothing here puts it right');
	});

	it('names the non-action on every code that offers nothing', () => {
		// §17.5's rule, applied exhaustively rather than to the one case it was
		// written for: a row with no clause and no button says only "not sent",
		// which is the chip restated.
		for (const code of [...NOT_SENT_CODES, 'quantum', '']) {
			const copy = notSent(code);
			if (copy.action === 'none') expect(copy.nonAction).toBeTruthy();
		}
	});

	it('claims no non-action where the offered action genuinely resolves it', () => {
		// Saying "there is nothing you can do" beside a control that works is its
		// own kind of dishonesty. These are the codes whose action is the fix.
		for (const code of ['expired', 'search_failed', 'not_found', 'no_indexer_service']) {
			expect(notSent(code).nonAction).toBeUndefined();
		}
		// And the counter-case, which keeps the rule from collapsing into "an
		// action means no sentence": searching again does not FIX a withdrawn
		// release, it is how you find out, and Services shows now rather than then.
		expect(notSent('no_longer_offered').nonAction).toBeTruthy();
		expect(notSent('service_unavailable').nonAction).toBeTruthy();
	});

	it('points at Services only where UsArr’s own configuration is at fault', () => {
		// §17.5: Open Services is a dead end where Services will correctly show
		// the service as reachable. So it is not offered on the codes that
		// describe Prowlarr's settings or an indexer's answer.
		const services = NOT_SENT_CODES.filter((c) => notSent(c).action === 'open-services');
		expect(services.sort()).toEqual(['no_indexer_service', 'service_unavailable'].sort());
		for (const code of ['no_download_client', 'no_indexers', 'grab_failed']) {
			expect(notSent(code).action).not.toBe('open-services');
		}
	});

	it('offers nothing to aim at a row with no release name', () => {
		// A nameless row has no query a fresh search could carry, so the action
		// that WOULD have been offered for its code is withdrawn and the sentence
		// says why. A control that cannot be aimed is the fake action §17.5 bans
		// wearing different clothes.
		const nameless = notSent('not_found', false);
		expect(notSent('not_found', true).action).toBe('search-again');
		expect(nameless.action).toBe('none');
		expect(nameless.nonAction).toContain('nothing to search for');
	});

	it('explains an empty title rather than rendering a blank or inventing one', () => {
		// The empty string is a FACT on this arm — release_candidate is swept 25
		// minutes after the search — so it gets a sentence, not NOTHING.empty's
		// em dash and certainly not a guessed name.
		expect(GRAB_MISSING_TITLE_NOTE).toBeTruthy();
		expect(GRAB_MISSING_TITLE_NOTE).not.toBe('—');
		expect(GRAB_MISSING_TITLE_NOTE).toContain('already gone');
	});

	it('renders a code it has never heard of rather than dropping the row', () => {
		// errorcodes.go's vocabulary is a wire contract that can grow, so an
		// unknown code is a state to render honestly, not an impossibility.
		const copy = notSent('quantum');
		expect(copy.label).toBe('not sent');
		expect(copy.detail).toBeTruthy();
		expect(copy.action).toBe('none');
		expect(copy.nonAction).toContain('newer than this screen');
	});

	it('gives the two handed-over states no action at all, permanently', () => {
		// The only control that would fit either is one that sends the release
		// again, which is precisely what produces two copies of a 68 GB release.
		for (const value of [OUTCOME_SENT, OUTCOME_SENT_UNKNOWN, 'pending', undefined]) {
			expect(grabOutcome(value).action).toBe('none');
			expect(grabOutcome(value).nonAction).toBeUndefined();
		}
	});
});

describe('searchEmptyState', () => {
	// §9.6 requires three distinct messages and §17.5 scopes the middle one by
	// media type (SW-08). The three are asserted as three rather than by their
	// wording, because the failure this guards is two of them collapsing into one.
	const answering = { answered: 9, total: 9 };

	it('separates “not searched yet” from “searched and got nothing”', () => {
		const idle = searchEmptyState({ submitted: '', typeId: 'basic', answered: 0 });
		const nothing = searchEmptyState({ submitted: 'norah jones', typeId: 'basic', ...answering });
		expect(idle.title).toBe(EMPTY_IDLE_TITLE);
		expect(nothing.title).not.toBe(idle.title);
		expect(nothing.text).not.toBe(idle.text);
	});

	it('separates “nothing matched” from “nobody answered”', () => {
		// Prowlarr answers 200 with [] for both, so a screen that renders one
		// message over the pair tells a user their query is wrong at the exact
		// moment their indexers are down.
		const matched = searchEmptyState({ submitted: 'dune', typeId: 'movie', ...answering });
		const silent = searchEmptyState({ submitted: 'dune', typeId: 'movie', answered: 0, total: 9 });
		expect(silent.title).toBe('No indexer answered');
		expect(silent.title).not.toBe(matched.title);
		expect(silent.text).toContain('has not actually been tested');
	});

	it('names thin indexer coverage for music and for books, and only for those', () => {
		// SW-08's whole point: 403 of Prowlarr's 543 definitions are private and
		// the trackers that carry these two are invite-only, so an empty answer
		// from indexers that ANSWERED is usually coverage, not the query.
		const music = searchEmptyState({ submitted: 'norah jones', typeId: 'music', ...answering });
		const book = searchEmptyState({ submitted: 'gaiman', typeId: 'book', ...answering });
		expect(music.title).toBe('No music releases matched');
		expect(book.title).toBe('No book releases matched');
		for (const copy of [music, book]) {
			expect(copy.text).toContain('invite-only');
			expect(copy.text).toContain('likelier explanation than your query');
		}
	});

	it('points a film or an episode search at the query instead, which is the opposite advice', () => {
		const movie = searchEmptyState({ submitted: 'dune', typeId: 'movie', ...answering });
		const tv = searchEmptyState({ submitted: 'severance', typeId: 'tv', ...answering });
		for (const copy of [movie, tv]) {
			expect(copy.text).toContain('the query is the likelier explanation');
			expect(copy.text).not.toContain('invite-only');
		}
		expect(movie.text).toContain('films');
		expect(tv.text).toContain('episodes and series');
	});

	it('names no cause at all for Basic, because the type is what would have named one', () => {
		// The selector is set to "do not narrow this", so neither the coverage
		// sentence nor the query sentence is a claim the data supports. It gets
		// the mechanical fact — UsArr sends the query unchanged — and nothing else.
		const basic = searchEmptyState({ submitted: 'dune', typeId: 'basic', ...answering });
		expect(basic.text).not.toContain('invite-only');
		expect(basic.text).not.toContain('likelier explanation');
		expect(basic.text).toContain('sends your query unchanged');
	});

	it('lets a type it does not recognise fall back rather than claim a cause', () => {
		expect(searchEmptyState({ submitted: 'x', typeId: 'comics', ...answering }).text).toBe(
			searchEmptyState({ submitted: 'x', typeId: 'basic', ...answering }).text
		);
	});

	it('gives “nobody answered” priority over the type, on every type', () => {
		// Neither scoped sentence is true of a fan-out nobody answered: the query
		// was never put to an indexer and coverage was never exercised.
		for (const typeId of SEARCH_TYPES.map((t) => t.id)) {
			const copy = searchEmptyState({ submitted: 'x', typeId, answered: 0, total: 9 });
			expect(copy.title).toBe('No indexer answered');
			expect(copy.text).not.toContain('invite-only');
		}
	});

	it('states the counts without inventing a denominator it was not given', () => {
		expect(searchEmptyState({ submitted: 'x', typeId: 'music', answered: 4 }).text).toContain(
			'4 indexers answered'
		);
		expect(
			searchEmptyState({ submitted: 'x', typeId: 'music', answered: 1, total: 1 }).text
		).toContain('1 of 1 indexer answered');
	});

	it('quotes the query back on the states whose advice is about the query', () => {
		expect(searchEmptyState({ submitted: 'dune', typeId: 'movie', ...answering }).text).toContain(
			'“dune”'
		);
	});
});

describe('correlatedFailure', () => {
	// §17.5: "naming the non-action beats offering a fake one". The screen used
	// to print the per-indexer banner N times when all N legs came back the same
	// way, so the one fact worth having — that the N are one condition — was on
	// screen nowhere.
	const leg = (over: Partial<IndexerLeg> = {}): IndexerLeg => ({
		name: 'Indexer',
		status: 'failed',
		answered: false,
		reason: 'indexer query failed: dial tcp: lookup indexer.example: no such host',
		...over
	});

	const all = (n: number, over: Partial<IndexerLeg> = {}) =>
		Array.from({ length: n }, (_, i) => leg({ name: `Indexer ${i + 1}`, ...over }));

	it('collapses N identical failures into one diagnosis naming the count', () => {
		const diag = correlatedFailure(all(9), 9);
		expect(diag?.kind).toBe('shared_fault');
		expect(diag?.indexers).toBe(9);
		expect(diag?.title).toBe('All 9 indexers came back with the same error');
	});

	it('carries the upstream sentence exactly once', () => {
		const diag = correlatedFailure(all(9), 9);
		expect(diag?.verbatim).toBe(leg().reason);
	});

	it('refuses to fire when one indexer answered', () => {
		// One answer makes this a partial result with a gap, which is what the
		// per-indexer banners already say correctly.
		const legs = [...all(8), leg({ answered: true, status: 'ok', reason: undefined })];
		expect(correlatedFailure(legs, 9)).toBeUndefined();
	});

	it('refuses to fire on a single indexer, because one sample is not a correlation', () => {
		// There is also nothing to collapse: one banner is already one banner.
		expect(correlatedFailure(all(1), 1)).toBeUndefined();
	});

	it('refuses to fire on a partially reported fan-out', () => {
		// Three legs that happened to fail first are not "every indexer". The
		// server's own total is the denominator; where it disagrees with the legs
		// in hand, nothing is concluded.
		expect(correlatedFailure(all(3), 9)).toBeUndefined();
	});

	it('refuses to fire on mixed statuses, because that is several conditions', () => {
		// Deliberately two statuses that BOTH map to a class, and one shared
		// reason, so the only thing standing between this input and a wrong
		// verdict is the status-agreement check itself. A mix of `failed` with
		// anything else would have been caught by the shared-reason rule instead
		// and would have left this guard untested.
		const legs = [
			...all(5, { status: 'timed_out', reason: 'same' }),
			...all(4, { status: 'blocked', reason: 'same' })
		];
		expect(correlatedFailure(legs, 9)).toBeUndefined();
	});

	it('refuses to call N different errors one fault', () => {
		// Same status, different messages: that is N errors, which is precisely
		// not the thing being detected.
		const legs = all(3).map((l, i) => ({ ...l, reason: `error ${i}` }));
		expect(correlatedFailure(legs, 3)).toBeUndefined();
	});

	it('refuses the unclassified remainder with no reason at all to correlate on', () => {
		expect(correlatedFailure(all(4, { reason: undefined }), 4)).toBeUndefined();
		expect(correlatedFailure(all(4, { reason: '   ' }), 4)).toBeUndefined();
	});

	it('classifies off the server’s status field, not off its prose', () => {
		// internal/releases/search.go sets exactly one status per leg and each
		// means something different about who is at fault. Nothing here reads an
		// error message to decide what happened.
		const kinds = (
			['timed_out', 'breaker_open', 'blocked', 'disabled', 'unsupported', 'not_found'] as const
		).map((status) => correlatedFailure(all(4, { status }), 4)?.kind);
		expect(kinds).toEqual([
			'timeout',
			'breaker',
			'blocked',
			'disabled',
			'unsupported',
			'unknown_ids'
		]);
	});

	it('ignores a status it does not know rather than diagnosing one', () => {
		expect(correlatedFailure(all(4, { status: 'quantum' }), 4)).toBeUndefined();
	});

	it('never names a cause the evidence only permits', () => {
		// §17.5's example is a host-level resolution failure inside Prowlarr's own
		// container. It is one of several conditions that produce exactly this
		// evidence, so printing it as the verdict would be an invented status.
		const diag = correlatedFailure(all(9), 9);
		const copy = `${diag?.title} ${diag?.text} ${diag?.nonAction}`.toLowerCase();
		for (const invented of ['dns', 'resolution', 'container', 'firewall', 'offline', 'crashed']) {
			expect(copy).not.toContain(invented);
		}
		// What it does say is what was observed, plus how far it reaches.
		expect(diag?.text).toContain('one fault rather than 9');
		expect(diag?.text).toContain('the path they share');
	});

	it('hedges every inference it draws, on every class that draws one', () => {
		// "every indexer failed the same way, which usually means X" is honest;
		// asserting X is not.
		for (const status of ['failed', 'timed_out', 'breaker_open']) {
			const diag = correlatedFailure(all(6, { status }), 6);
			expect(`${diag?.text}`).toMatch(/usually/);
		}
	});

	it('names the non-action rather than offering a button that cannot act', () => {
		// The whole rule. A screen that reasons its way to a correct diagnosis and
		// then hands the user two buttons that cannot act on it is worse than one
		// that says so.
		for (const status of [
			'failed',
			'timed_out',
			'breaker_open',
			'blocked',
			'disabled',
			'unsupported'
		]) {
			const diag = correlatedFailure(all(6, { status }), 6);
			expect(diag?.nonAction).toBeTruthy();
		}
	});

	it('offers no action at all where nothing on this screen can move it', () => {
		for (const status of ['breaker_open', 'blocked', 'disabled', 'unsupported']) {
			expect(correlatedFailure(all(6, { status }), 6)?.action).toBe('none');
		}
	});

	it('offers the one action that does resolve it, and then claims no non-action', () => {
		// Unknown ids are a fact about the REQUEST — a shared link or a remembered
		// selection from a different Prowlarr — and clearing the scope fixes it.
		// Saying "there is nothing you can do" beside a button that works would be
		// its own kind of dishonesty.
		const diag = correlatedFailure(all(6, { status: 'not_found' }), 6);
		expect(diag?.action).toBe('clear-scope');
		expect(diag?.nonAction).toBe('');
	});

	it('never offers Retry, on any class', () => {
		// Not in the union, and not in the words. The screen's word for asking
		// again is Search again, which is a fresh fan-out rather than a re-send.
		for (const status of ['failed', 'timed_out', 'breaker_open', 'blocked', 'not_found']) {
			const diag = correlatedFailure(all(6, { status }), 6);
			expect(diag?.action).not.toBe('retry');
			expect(`${diag?.title} ${diag?.text} ${diag?.nonAction}`.toLowerCase()).not.toContain(
				'retry'
			);
		}
	});

	it('leaves the per-leg words to the caller where the legs disagree on them', () => {
		// A blocked or breaker-open set carries its own unblock time per line, so
		// dropping the lines would drop real data.
		const legs = all(3, { status: 'blocked' }).map((l, i) => ({
			...l,
			reason: `blocked until 0${i}`
		}));
		const diag = correlatedFailure(legs, 3);
		expect(diag?.kind).toBe('blocked');
		expect(diag?.verbatim).toBe('');
		expect(legReasons(legs)).toBe(
			'Indexer 1: blocked until 00\nIndexer 2: blocked until 01\nIndexer 3: blocked until 02'
		);
	});

	it('falls back to the status when a leg has no reason to print', () => {
		expect(legReasons([leg({ name: 'A', reason: undefined, status: 'blocked' })])).toBe(
			'A: blocked'
		);
	});
});

describe('the banned vocabulary', () => {
	// Every user-facing string this module ships, held against §17.5's ban list.
	// This is the guard: an edit that reintroduces "succeeded" or wires a
	// "failed" chip fails here rather than passing review.
	//
	// ⚠️ THE NOT-SENT ARM IS IN THE CORPUS RATHER THAN EXEMPT FROM IT, AND THAT
	// IS THE DECISION WORTH RECORDING. §17.5 permits state 3 to be worded as "it
	// did not happen" — UsArr genuinely knows the release never left the process
	// — so "failed" would have been defensible there and nowhere else. Carving
	// the exception was refused for the reason §17.5 gives for preferring `Search
	// again` to `Retry`: a guard over words cannot see context, so the choice is
	// between an absolute guard and one with exceptions, and a guard with
	// exceptions is one people argue with rather than obey. Every code's clause,
	// its non-action and the nameless variant are all held to the full list.
	const strings = [
		SEARCH_TYPE_NOTE,
		THIN_COVERAGE_NOTE,
		KNOWLEDGE_STOPS_NOTE,
		NOT_SENT_NOTE,
		GRAB_MISSING_TITLE_NOTE,
		...[OUTCOME_SENT, OUTCOME_SENT_UNKNOWN, 'unknown'].flatMap((o) => {
			const copy = grabOutcome(o);
			return [copy.label, copy.detail ?? ''];
		}),
		// Every not-sent code, both with a title and without, so the nameless
		// variant and the unrecognised fallback are both covered. The CODE strings
		// themselves are deliberately not in the corpus — `search_failed` is a wire
		// identifier from errorcodes.go and never reaches a user.
		...[...NOT_SENT_CODES, 'quantum', ''].flatMap((code) =>
			[true, false].flatMap((hasTitle) => {
				const copy = grabOutcome(OUTCOME_NOT_SENT, { errorCode: code, hasTitle });
				return [copy.label, copy.detail ?? '', copy.nonAction ?? ''];
			})
		)
	].filter((s) => s.length > 0);

	it('is holding the strings it thinks it is holding', () => {
		// A guard that matches nothing is indistinguishable from no guard. The
		// floor catches a helper that started returning empty copy, and the two
		// pins catch a corpus that quietly stopped reaching the not-sent arm.
		expect(strings.some((s) => s.includes('the listing had expired'))).toBe(true);
		expect(strings.some((s) => s.includes('nothing here puts it right'))).toBe(true);
		expect(strings.length).toBeGreaterThan(40);
	});

	it.each(FORBIDDEN_OUTCOME_WORDS)('never says “%s”', (word) => {
		for (const value of strings) {
			expect(value.toLowerCase()).not.toContain(word);
		}
	});

	it('never asserts that a grab did not happen', () => {
		// The one unknowable claim. Prowlarr adds the release to the download
		// client BEFORE the step that failed for the owner, and never rolls
		// back, so a 5xx can cover an operation that already partly succeeded.
		//
		// It stays absolute across the not-sent arm too, where the claim IS
		// knowable: the phrases below are banned wordings rather than banned
		// facts, and the not-sent copy states the same fact more precisely — the
		// release never left, and here is what stopped it.
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

	it('states both halves of the gap, so neither reading is available', () => {
		// The line has TWO facts to carry now and gets them wrong in opposite
		// directions if it carries one. The old wording said the not-sent arm was
		// unbuilt, which is no longer true — the union with audit_log ships. The
		// obvious replacement, "every grab is listed", is false the other way:
		// three of handleGrab's seven pre-dispatch returns write no audit row by
		// design, because none of them ever named a release.
		expect(NOT_SENT_NOTE).toContain('listed here too');
		expect(NOT_SENT_NOTE).toContain('not all of them');
		// And it must not have kept the retired claim.
		expect(NOT_SENT_NOTE).not.toContain('not listed here yet');
		expect(NOT_SENT_NOTE.toLowerCase()).not.toContain('still being built');
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

/**
 * The same two-region rule applied to the strings this module exports for the
 * SEARCH half of the screen.
 *
 * The block near the top of this file holds the grab-related exports against the
 * full ban list. These are not grab copy: an empty state and a fan-out diagnosis
 * are statements about a search, which UsArr watches from the first indexer to
 * the closing report — so they take the same subset the non-grab markup takes,
 * for the reason recorded above `NEVER_ANYWHERE_ON_THIS_SCREEN`. They still take
 * something: `retry` and the three words that assert bytes are moving are a
 * claim about a download wherever they appear, and this copy is rendered on the
 * same screen as the Grab button.
 */
/**
 * Every user-facing string the scope picker can produce.
 *
 * It is built rather than listed, so a new branch inside one of these helpers is
 * covered the day it is written instead of the day somebody remembers to extend
 * an array here.
 */
const SCOPE_COPY: string[] = (() => {
	const base = {
		instanceId: 1,
		instanceName: 'Prowlarr',
		indexerId: 1,
		name: 'Indexer',
		protocol: 'usenet',
		privacy: 'public',
		enabled: true,
		priority: 25,
		supportsSearch: true,
		searchTypes: ['search'],
		categories: [],
		fetchedAt: ''
	};
	const named = [
		{ id: 1, name: 'Alpha' },
		{ id: 2, name: 'Beta' },
		{ id: 3, name: 'Gamma' },
		{ id: 4, name: 'Delta' }
	];
	// The service NAME is rendered upstream data — the user's own label for a
	// configured service — so the corpus uses neutral names on purpose. §17.5's
	// ban is over UsArr's own sentences and has never reached a service or an
	// indexer name; a user who calls their Prowlarr "Downloading" is not the
	// thing this guard is for.
	const service = {
		instanceId: 1,
		name: 'Prowlarr',
		kind: 'prowlarr',
		enabled: true,
		status: 'ok',
		message: '3 indexers from "Prowlarr"',
		action: '',
		fetchedAt: '2026-08-16T12:00:00Z',
		indexerCount: 3
	};
	return [
		unavailableReason({ ...base, enabled: false }),
		unavailableReason({ ...base, supportsSearch: false }),
		unavailableReason(base),
		PRIORITY_NOTE,
		indexerFacts(base),
		indexerFacts({ ...base, priority: 0 }),
		// The copy that changes meaning with a second indexer service. Every arm
		// of each helper, so the one-service wording and the two-service wording
		// are both held to the list.
		...[0, 1, 2, 3].flatMap((count) => [
			indexerPickerLegend('Prowlarr', count),
			pickerScopeNote(count),
			clearScopeLabel(count)
		]),
		serviceFacts(service),
		serviceFacts({ ...service, indexerCount: 1 }),
		serviceFacts({ ...service, indexerCount: 0 }),
		serviceFacts({ ...service, fetchedAt: undefined }),
		serviceFacts({ ...service, enabled: false }),
		...[0, 1, 2, 4].flatMap((n) =>
			[0, 1, 2, 4].flatMap((m) =>
				// With and without a service, and with one service and with several:
				// the sentence has a different shape in each and all of them ship.
				[undefined, { name: 'Prowlarr', total: 1 }, { name: 'Prowlarr', total: 2 }].map(
					(withService) =>
						scopeSummary({
							indexers: named.slice(0, n),
							categories: named.slice(0, m),
							knownIndexers: 9,
							service: withService
						})
				)
			)
		)
	].filter((s) => s.length > 0);
})();

describe('the banned vocabulary, over the search-side copy', () => {
	const strings = [
		...['basic', 'movie', 'tv', 'music', 'book', 'comics'].flatMap((typeId) =>
			[
				{ submitted: '', typeId, answered: 0 },
				{ submitted: 'q', typeId, answered: 0, total: 9 },
				{ submitted: 'q', typeId, answered: 9, total: 9 }
			].flatMap((input) => {
				const copy = searchEmptyState(input);
				return [copy.title, copy.text];
			})
		),
		...['failed', 'timed_out', 'breaker_open', 'blocked', 'disabled', 'unsupported', 'not_found']
			.map((status) =>
				correlatedFailure(
					Array.from({ length: 4 }, (_, i) => ({
						name: `Indexer ${i + 1}`,
						status,
						answered: false,
						reason: 'shared reason'
					})),
					4
				)
			)
			.flatMap((diag) => [diag?.title ?? '', diag?.text ?? '', diag?.nonAction ?? '']),
		// The scope picker's own copy, which lives in `$lib/indexercatalog` for the
		// same reason the rest of this does: it renders on THIS screen, beside the
		// Grab button, so §17.5's ban reaches it. `indexercatalog.test.ts` asserts
		// what these strings mean; this asserts what they may not say.
		...SCOPE_COPY
	]
		// The one deliberate empty string in the corpus is `unknown_ids.nonAction`,
		// which is empty BECAUSE that class has an action that works. Dropping
		// empties here rather than asserting them away keeps the guard from
		// silently passing on a helper that returned nothing at all — the count
		// below is what catches that.
		.filter((s: string) => s.length > 0);

	it('is holding the strings it thinks it is holding', () => {
		// A guard that matches nothing is indistinguishable from no guard, so the
		// corpus is pinned to something only this copy says, and to a floor that a
		// helper returning `undefined` for everything would fall through.
		expect(strings.some((s) => s.includes('invite-only'))).toBe(true);
		expect(strings.some((s) => s.includes('came back with the same error'))).toBe(true);
		// And something only the scope picker says, so widening the corpus to it
		// cannot silently become a no-op.
		expect(strings.some((s) => s.includes('the lower number wins'))).toBe(true);
		// And something only the two-service copy says, so the arms added for the
		// one-search-one-service fix cannot silently fall out of the corpus.
		expect(strings.some((s) => s.includes('is not asked'))).toBe(true);
		expect(strings.some((s) => s.includes('every indexer in this service'))).toBe(true);
		expect(strings.length).toBeGreaterThan(50);
	});

	it.each(NEVER_ANYWHERE_ON_THIS_SCREEN)('never says “%s”', (word) => {
		for (const value of strings) {
			expect(value.toLowerCase()).not.toContain(word);
		}
	});

	it('never asserts a cause outside what UsArr observed', () => {
		// UsArr talks to Prowlarr and Prowlarr talks to the indexers. It sees the
		// near half and infers nothing about the far half, so no sentence here may
		// name the mechanism §17.5 gives only as an example of the class.
		for (const value of strings) {
			for (const invented of ['dns', 'resolution', 'container', 'firewall', 'crashed']) {
				expect(value.toLowerCase()).not.toContain(invented);
			}
		}
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

/**
 * THE `?q=` CONTRACT, which §17.4 fixes twice — the `Search indexers →` action
 * on an incomplete result row, and the zero-results state — and which Home now
 * also uses. It is one function so the three call sites cannot encode a query
 * three ways; the Requests screen's `onMount` reads `?q=`, trims it and runs the
 * search, so anything this builds must survive a middle-click into a new tab.
 */
describe('requestsSearchHref', () => {
	it('carries the query on the parameter Requests reads', () => {
		expect(requestsSearchHref('/requests', 'dune')).toBe('/requests?q=dune');
	});

	it('preserves a configured base path rather than rebuilding one', () => {
		expect(requestsSearchHref('/usarr/requests', 'dune')).toBe('/usarr/requests?q=dune');
	});

	it('encodes what would otherwise change the URL', () => {
		// A release name is arbitrary text: `&`, `#` and a space all break a
		// hand-built link, and `#` would drop everything after it.
		expect(requestsSearchHref('/requests', 'S&M #2 live')).toBe('/requests?q=S%26M%20%232%20live');
	});

	it('drops the parameter entirely for an empty or blank query', () => {
		// `?q=` would make Requests echo a search for nothing back at the user
		// instead of showing its idle state.
		expect(requestsSearchHref('/requests', '')).toBe('/requests');
		expect(requestsSearchHref('/requests', '   ')).toBe('/requests');
	});

	it('trims, so a pasted query does not travel with its whitespace', () => {
		expect(requestsSearchHref('/requests', '  dune  ')).toBe('/requests?q=dune');
	});
});
