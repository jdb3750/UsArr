import { describe, expect, it } from 'vitest';
import {
	HEALTH_UNREAD,
	completenessNote,
	libraryStates,
	toCompleteness,
	toLibrary,
	type Library
} from './libraries';

/**
 * THE CONTENT-FILTER SHORTFALL, ON THE SCREEN.
 *
 * The measurement is the server's; what is pinned here is the RENDERING, and the
 * rendering carries the whole point of the feature. A shortfall the user cannot
 * see is worth exactly as much as no check at all, and an unmeasured library
 * that renders as fine recreates the defect the check exists to close.
 *
 * Three properties, asserted separately because they fail separately:
 *
 *   1. A shortfall says the two numbers, in words, on the row.
 *   2. `unverified` says so OUT LOUD and never renders as silence.
 *   3. `complete` renders NOTHING, and that silence is the same silence an
 *      unmeasured library gets — which is fine, and is why (2) has to be loud.
 */

function wire(over: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		id: 2,
		name: 'Ebooks',
		slug: 'ebooks',
		kind: 'book',
		sort_order: 5,
		enabled: true,
		include_in_search: true,
		item_count: 3,
		sources: [
			{
				id: 2,
				service_instance_id: 1,
				service_name: 'BookOrbit',
				service_kind: 'bookorbit',
				container_kind: 'remote_library',
				container_ref: '12',
				is_metadata_authority: true
			}
		],
		...over
	};
}

function parse(over: Record<string, unknown> = {}): Library {
	const l = toLibrary(wire(over));
	if (l === undefined) throw new Error('the base fixture must parse');
	return l;
}

/** The marks a row carries, as `{word, detail}` pairs. */
function marks(l: Library) {
	return libraryStates(l, HEALTH_UNREAD).map((m) => ({ word: m.word, detail: m.detail }));
}

describe('the completeness verdict, parsed', () => {
	it('carries the two counts and the age of the measurement', () => {
		expect(
			toCompleteness({
				state: 'shortfall',
				total_items: 412,
				visible_items: 389,
				hidden_items: 23,
				checked_at: '2026-08-19T10:00:00Z'
			})
		).toEqual({
			state: 'shortfall',
			total: 412,
			visible: 389,
			hidden: 23,
			checkedAt: '2026-08-19T10:00:00Z'
		});
	});

	it('drops a count that arrives under unverified', () => {
		// The server omits them; if one arrives anyway it is a claim the label
		// denies, so it does not reach a renderer.
		const got = toCompleteness({
			state: 'unverified',
			total_items: 0,
			reason: 'the route refused'
		});
		expect(got).toEqual({ state: 'unverified', reason: 'the route refused' });
	});

	it('refuses a state it does not know rather than guessing at the nearest one', () => {
		// ⚠️ THE TEMPTING BUG IS A DEFAULT. A fourth state written by some later
		// adapter is a verdict this build cannot read, and the only honest
		// rendering of an unreadable measurement is no measurement — never
		// `complete`.
		expect(toCompleteness({ state: 'probably fine', total_items: 9 })).toBeUndefined();
		expect(toCompleteness({ state: '' })).toBeUndefined();
		expect(toCompleteness(undefined)).toBeUndefined();
		expect(toCompleteness('complete')).toBeUndefined();
	});

	it('is absent on a row the server did not measure', () => {
		expect(parse().completeness).toBeUndefined();
	});
});

describe('the shortfall on a row', () => {
	it('says what the library holds and what the account can see', () => {
		const l = parse({
			completeness: { state: 'shortfall', total_items: 412, visible_items: 389, hidden_items: 23 }
		});
		expect(marks(l)).toContainEqual({
			word: 'Some books are hidden',
			detail: 'this library holds 412 books; the service account can see 389'
		});
	});

	it('groups the numbers, because a five-figure library is the ordinary case', () => {
		const l = parse({
			completeness: { state: 'shortfall', total_items: 12400, visible_items: 9000 }
		});
		expect(marks(l)[0].detail).toBe(
			'this library holds 12,400 books; the service account can see 9,000'
		);
	});

	it('is amber, and carries the age of the measurement', () => {
		const l = parse({
			completeness: {
				state: 'shortfall',
				total_items: 4,
				visible_items: 1,
				checked_at: '2026-08-19T10:00:00Z'
			}
		});
		const [mark] = libraryStates(l, HEALTH_UNREAD);
		expect(mark.tone).toBe('warn');
		// ⚠️ WITHOUT THE STAMP A STORED MEASUREMENT READS AS A LIVE FACT. The
		// verdict is from the last import, which may have been days ago.
		expect(mark.at).toBe('2026-08-19T10:00:00Z');
	});
});

describe('unverified', () => {
	it('says so on the row rather than rendering as silence', () => {
		// ⚠️ THIS IS THE TEST THE WHOLE DESIGN EXISTS FOR. The check rests on an
		// upstream route carrying no permission guard. If that changes, every
		// verdict becomes this one — and a screen that rendered it as nothing
		// would report a service that had stopped being measurable as a service
		// with nothing wrong.
		const l = parse({
			completeness: { state: 'unverified', reason: 'BookOrbit refused UsArr the statistics' }
		});
		expect(marks(l)).toContainEqual({
			word: 'Completeness unverified',
			detail: 'BookOrbit refused UsArr the statistics'
		});
	});

	it('still says something when the server sent no reason', () => {
		const l = parse({ completeness: { state: 'unverified' } });
		expect(marks(l)[0].word).toBe('Completeness unverified');
		expect(marks(l)[0].detail).not.toBe('');
	});

	it('is grey, not amber: nothing is broken and nothing is known to be missing', () => {
		const l = parse({ completeness: { state: 'unverified' } });
		expect(libraryStates(l, HEALTH_UNREAD)[0].tone).toBe('none');
	});

	it('never claims a number', () => {
		const l = parse({ completeness: { state: 'unverified', reason: 'r' } });
		for (const mark of libraryStates(l, HEALTH_UNREAD)) {
			expect(`${mark.word} ${mark.detail ?? ''}`).not.toMatch(/\d/);
		}
	});
});

describe('complete', () => {
	it('renders no mark at all', () => {
		// The standing invariant of this column: nothing on this screen renders a
		// positive claim. `complete` is a real measurement and still gets no tick,
		// because a tick would fire on nearly every row.
		const l = parse({
			completeness: { state: 'complete', total_items: 12, visible_items: 12, hidden_items: 0 }
		});
		expect(marks(l)).toEqual([]);
	});

	it('is indistinguishable from an unmeasured library on the row, which is why unverified is loud', () => {
		const measured = parse({ completeness: { state: 'complete', total_items: 3 } });
		const unmeasured = parse();
		expect(marks(measured)).toEqual(marks(unmeasured));
	});
});

describe('the sentence above the table', () => {
	it('is empty when nothing was measured, and when everything measured was fine', () => {
		expect(completenessNote([parse()])).toBe('');
		expect(completenessNote([parse({ completeness: { state: 'complete', total_items: 1 } })])).toBe(
			''
		);
	});

	it('names the fix, and points off this screen because the fix is off this screen', () => {
		const note = completenessNote([
			parse({ completeness: { state: 'shortfall', total_items: 4, visible_items: 1 } })
		]);
		// The three things §13 asks of a copy string: the component, the symptom,
		// the next action. No control on this screen can widen a content filter.
		expect(note).toContain('content filter');
		expect(note).toContain('not in UsArr');
		expect(note).toContain('next import');
	});

	it('counts the affected libraries rather than repeating itself per row', () => {
		const note = completenessNote([
			parse({ completeness: { state: 'shortfall', total_items: 4, visible_items: 1 } }),
			parse({ completeness: { state: 'shortfall', total_items: 9, visible_items: 2 } })
		]);
		expect(note).toContain('2 libraries');
	});

	it('reports unverified separately, and not as a failure', () => {
		const note = completenessNote([parse({ completeness: { state: 'unverified' } })]);
		expect(note).toContain('could not check');
		expect(note).toContain('rather than being counted as complete');
	});

	it('says both when both are true', () => {
		const note = completenessNote([
			parse({ completeness: { state: 'shortfall', total_items: 4, visible_items: 1 } }),
			parse({ completeness: { state: 'unverified' } })
		]);
		expect(note).toContain('content filter');
		expect(note).toContain('could not check');
	});
});
