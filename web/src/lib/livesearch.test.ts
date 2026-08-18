import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './api';
import {
	ANNOUNCE_SETTLE_MS,
	announcementFor,
	LIVE_DEBOUNCE_MS,
	LIVE_SEARCH_LIMIT,
	livePlan,
	LiveSearch,
	type LiveRegion
} from './livesearch';
import type { SearchItem, SearchResults } from './search';

/**
 * FILTER-AS-YOU-TYPE, AND ABOVE ALL THE RACE.
 *
 * The test this file exists for is `renders the newest answer when an older
 * one resolves last`. Everything else here is a boundary; that one is the
 * defect the feature would otherwise ship with, because it cannot be found by
 * using the app — it needs the older read to be SLOWER than the newer one,
 * which on a healthy local endpoint almost never happens and on a loaded box
 * happens all day. So the two answers are resolved by hand, in the wrong order,
 * and `fires when the sequence guard is removed` re-runs the same scenario
 * against a copy of the controller that has no guard, to prove the assertion
 * is capable of failing.
 */

function item(id: number, title: string): SearchItem {
	return { id, kind: 'comic', mediaType: 'comics', title, haveCount: 0, wantCount: 0 };
}

function answer(query: string, titles: string[], limit = LIVE_SEARCH_LIMIT): SearchResults {
	return {
		query,
		limit,
		truncated: false,
		items: titles.map((t, i) => item(i + 1, t))
	};
}

/** One search that never settles on its own. `resolve` and `reject` are the
 * test's, so the order two answers land in is chosen rather than raced for. */
interface Pending {
	query: string;
	signal: AbortSignal;
	resolve(results: SearchResults): void;
	reject(error: unknown): void;
}

function harness() {
	const pending: Pending[] = [];
	const seen: { region: LiveRegion; announcement: string }[] = [];
	const live = new LiveSearch({
		search: (query, signal) =>
			new Promise<SearchResults>((resolve, reject) => {
				pending.push({ query, signal, resolve, reject });
			}),
		onchange: (region, announcement) => seen.push({ region, announcement })
	});
	return { live, pending, seen };
}

/** Let every already-settled promise run its `.then`. */
const drain = () => Promise.resolve().then(() => undefined);

describe('$lib/livesearch — what is sent', () => {
	it('livePlan: an empty or whitespace box sends nothing', () => {
		expect(livePlan('')).toEqual({ k: 'dormant' });
		expect(livePlan('   ')).toEqual({ k: 'dormant' });
		expect(livePlan('\t\n ')).toEqual({ k: 'dormant' });
	});

	it('livePlan: one one-character token is the floor searchquery.go declines', () => {
		// internal/store/searchquery.go's `empty()`: step 4 drops a one-character
		// final token from the keyword leg and step 5 keeps it off the trigram
		// leg, so no leg runs and http-api.md §6.6 answers 200 with no items.
		expect(livePlan('d')).toEqual({ k: 'floor', query: 'd' });
		expect(livePlan('  d  ')).toEqual({ k: 'floor', query: 'd' });
	});

	it('livePlan: two characters is above the floor, because the keyword leg runs', () => {
		expect(livePlan('du')).toEqual({ k: 'search', query: 'du' });
	});

	it('livePlan: the floor is a TOKEN, so `a b` is searched', () => {
		// Step 4 only ever drops the LAST token, so `"a"` still reaches the
		// keyword leg. Measuring the whole string would refuse a query the
		// server would have answered.
		expect(livePlan('a b')).toEqual({ k: 'search', query: 'a b' });
	});

	it('livePlan: an astral character is one character, not two', () => {
		expect(livePlan('𝑑')).toEqual({ k: 'floor', query: '𝑑' });
	});
});

describe('$lib/livesearch — the race', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('renders the newest answer when an older one resolves last', async () => {
		const { live, pending, seen } = harness();

		live.type('du');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(pending.map((p) => p.query)).toEqual(['du', 'dune']);

		// THE WRONG ORDER, DELIBERATELY: the newer read answers first, and the
		// older one lands after it. Without the sequence guard the older answer
		// is the last write and the region ends up showing results for `du`
		// under a box that says `dune`.
		pending[1].resolve(answer('dune', ['Dune']));
		await drain();
		expect(live.region).toEqual({ k: 'results', results: answer('dune', ['Dune']) });

		pending[0].resolve(answer('du', ['Dubliners', 'Duncan']));
		await drain();
		expect(live.region).toEqual({ k: 'results', results: answer('dune', ['Dune']) });

		// And it never even flickered through the stale one.
		const painted = seen.filter((s) => s.region.k === 'results');
		expect(painted).toHaveLength(1);
	});

	it('fires when the sequence guard is removed', async () => {
		// The same scenario against a controller whose guard is gone, so the
		// assertion above is known to be capable of failing. This is the shape
		// `LiveSearch.#run` would have without its `seq !== this.#seq` line.
		let region: LiveRegion = { k: 'dormant' };
		const pending: Pending[] = [];
		const search = (query: string) =>
			new Promise<SearchResults>((resolve, reject) => {
				pending.push({ query, signal: new AbortController().signal, resolve, reject });
			});
		const unguarded = async (query: string) => {
			const results = await search(query);
			region = { k: 'results', results };
		};

		void unguarded('du');
		void unguarded('dune');
		pending[1].resolve(answer('dune', ['Dune']));
		await drain();
		pending[0].resolve(answer('du', ['Dubliners', 'Duncan']));
		await drain();

		expect(region).toEqual({ k: 'results', results: answer('du', ['Dubliners', 'Duncan']) });
	});

	it('discards a stale answer even when the two queries are identical', async () => {
		// The echoed `query` cannot separate these two: both answers are
		// `isCurrent`. Only the sequence number can. Typing forward and then
		// deleting back is how two reads of one text are reached, since a
		// keystroke that leaves the trimmed query unchanged is not re-asked.
		const { live, pending } = harness();

		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		live.type('dune m');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(pending.map((p) => p.query)).toEqual(['dune', 'dune m', 'dune']);

		pending[2].resolve(answer('dune', ['Dune', 'Dune Messiah']));
		await drain();
		pending[0].resolve(answer('dune', ['stale']));
		await drain();

		expect(live.region).toEqual({
			k: 'results',
			results: answer('dune', ['Dune', 'Dune Messiah'])
		});
	});

	it('aborts the superseded read', async () => {
		const { live, pending } = harness();

		live.type('du');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(pending[0].signal.aborted).toBe(false);

		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(pending[0].signal.aborted).toBe(true);
		expect(pending[1].signal.aborted).toBe(false);
	});

	it('an aborted read rejecting is not drawn as a failure', async () => {
		// $lib/api wraps an AbortError into an ApiError at status 0, because
		// fetch gives it no way to tell that from an unreachable backend. The
		// sequence check has to catch it before the error arm can paint.
		const { live, pending } = harness();

		live.type('du');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);

		pending[1].resolve(answer('dune', ['Dune']));
		await drain();
		pending[0].reject(new ApiError('the UsArr backend could not be reached (aborted)', 0, '/x'));
		await drain();

		expect(live.region).toEqual({ k: 'results', results: answer('dune', ['Dune']) });
	});
});

describe('$lib/livesearch — the debounce', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('sends nothing before the debounce elapses', () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS - 1);
		expect(pending).toHaveLength(0);
		vi.advanceTimersByTime(1);
		expect(pending).toHaveLength(1);
	});

	it('a ten-character query typed at 200 ms a character costs seven reads', () => {
		// The arithmetic the constant's own note gives, executed. The first
		// character is below the floor, and the two space keys leave the trimmed
		// query unchanged, so three of the ten keystrokes ask nothing.
		const { live, pending } = harness();
		const text = 'the wire i';
		for (let i = 1; i <= text.length; i++) {
			live.type(text.slice(0, i));
			vi.advanceTimersByTime(200);
		}
		expect(pending.map((p) => p.query)).toEqual([
			'th',
			'the',
			'the w',
			'the wi',
			'the wir',
			'the wire',
			'the wire i'
		]);
	});

	it('the same ten characters with no space in them cost nine reads', () => {
		const { live, pending } = harness();
		const text = 'frierenxyz';
		for (let i = 1; i <= text.length; i++) {
			live.type(text.slice(0, i));
			vi.advanceTimersByTime(200);
		}
		expect(pending).toHaveLength(9);
	});

	it('a keystroke that does not change the trimmed query asks nothing', () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(200);
		live.type('dune ');
		vi.advanceTimersByTime(200);
		live.type('dune  ');
		vi.advanceTimersByTime(200);
		expect(pending.map((p) => p.query)).toEqual(['dune']);
	});

	it('but a failed read is re-asked, because what is on screen is not an answer', async () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		pending[0].reject(new ApiError('the local read failed', 500, '/api/v1/search'));
		await drain();
		live.type('dune ');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(pending).toHaveLength(2);
	});

	it('clearing the box and retyping the same text asks again', async () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		pending[0].resolve(answer('dune', ['Dune']));
		await drain();
		live.type('');
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(pending).toHaveLength(2);
	});

	it('a burst typed inside one debounce window costs one read', () => {
		const { live, pending } = harness();
		const text = 'the wire i';
		for (let i = 1; i <= text.length; i++) live.type(text.slice(0, i));
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(pending).toHaveLength(1);
		expect(pending[0].query).toBe(text);
	});

	it('never sends an empty or whitespace-only query', () => {
		// §6.1: empty or whitespace-only `q` is 400 bad_request. It is a known
		// error rather than a cheap read, so it is not sent.
		const { live, pending } = harness();
		live.type('   ');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS * 4);
		expect(pending).toHaveLength(0);
		expect(live.region).toEqual({ k: 'dormant' });
	});

	it('clearing the field empties the region on the same tick', async () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		pending[0].resolve(answer('dune', ['Dune']));
		await drain();
		expect(live.region.k).toBe('results');

		live.type('');
		expect(live.region).toEqual({ k: 'dormant' });
	});

	it('a pending debounce is cancelled by clearing the field', () => {
		const { live, pending } = harness();
		live.type('dune');
		live.type('');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS * 4);
		expect(pending).toHaveLength(0);
	});

	it('dispose cancels the timer and aborts what is in flight', () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		live.dispose();
		expect(pending[0].signal.aborted).toBe(true);
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS * 10);
		expect(pending).toHaveLength(1);
	});
});

describe('$lib/livesearch — what the region claims', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('keeps the previous answer on screen while the next read is in flight', async () => {
		// §7.2 Tier 0: no skeleton, no spinner, no fade-in. The answer to the
		// previous keystroke is true and still, which is better than an
		// indicator that says only `wait`.
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		pending[0].resolve(answer('dune', ['Dune']));
		await drain();

		live.type('dune m');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(live.region).toEqual({ k: 'results', results: answer('dune', ['Dune']) });
	});

	it('draws `nothing` only from an answer the server actually gave', async () => {
		const { live, pending } = harness();
		live.type('zzzz');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		expect(live.region).toEqual({ k: 'dormant' });
		pending[0].resolve(answer('zzzz', []));
		await drain();
		expect(live.region).toEqual({ k: 'nothing', query: 'zzzz' });
	});

	it('a one-character box claims nothing about the library', () => {
		const { live, pending } = harness();
		live.type('d');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS * 4);
		expect(pending).toHaveLength(0);
		expect(live.region).toEqual({ k: 'floor', query: 'd' });
	});

	it('a failed read is an error, not an empty library', async () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		pending[0].reject(new ApiError('the local read failed', 500, '/api/v1/search'));
		await drain();
		expect(live.region).toEqual({
			k: 'error',
			message: 'the local read failed',
			action: ''
		});
	});
});

describe('$lib/livesearch — what a screen reader hears', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('says nothing until the user has stopped', async () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		pending[0].resolve(answer('dune', ['Dune']));
		await drain();
		expect(live.announcement).toBe('');

		vi.advanceTimersByTime(ANNOUNCE_SETTLE_MS - 1);
		expect(live.announcement).toBe('');
		vi.advanceTimersByTime(1);
		expect(live.announcement).toBe('1 match in your library.');
	});

	it('does not read a count out on every keystroke', async () => {
		const { live, pending, seen } = harness();
		const text = 'the wire i';
		for (let i = 1; i <= text.length; i++) {
			live.type(text.slice(0, i));
			vi.advanceTimersByTime(200);
			const next = pending[pending.length - 1];
			if (next) {
				next.resolve(answer(text.slice(0, i), ['The Wire']));
				await drain();
			}
		}
		// Seven reads landed, and not one of them was spoken.
		expect(pending).toHaveLength(7);
		expect(seen.filter((s) => s.announcement !== '')).toHaveLength(0);

		vi.advanceTimersByTime(ANNOUNCE_SETTLE_MS);
		expect(seen.filter((s) => s.announcement !== '')).toHaveLength(1);
	});

	it('withdraws the announcement when typing resumes, so a repeat speaks again', async () => {
		const { live, pending } = harness();
		live.type('dune');
		vi.advanceTimersByTime(LIVE_DEBOUNCE_MS);
		pending[0].resolve(answer('dune', ['Dune']));
		await drain();
		vi.advanceTimersByTime(ANNOUNCE_SETTLE_MS);
		expect(live.announcement).toBe('1 match in your library.');

		live.type('dune ');
		expect(live.announcement).toBe('');
	});

	it('announcementFor: a full page says so rather than printing a bare count', () => {
		const full = answer('a', new Array(LIVE_SEARCH_LIMIT).fill('x'));
		expect(announcementFor({ k: 'results', results: full })).toBe(
			`First ${LIVE_SEARCH_LIMIT} matches in your library. More on the Search screen.`
		);
	});

	it('announcementFor: nothing to say about a box that has asked nothing', () => {
		expect(announcementFor({ k: 'dormant' })).toBe('');
		expect(announcementFor({ k: 'floor', query: 'd' })).toBe('');
	});
});
