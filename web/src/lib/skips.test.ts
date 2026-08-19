import { describe, expect, it } from 'vitest';
import {
	HEALTH_UNREAD,
	libraryStates,
	skippedNote,
	toLibrary,
	toSkips,
	type Library
} from './libraries';

/**
 * WHAT THE IMPORT READ AND DID NOT MAP, ON THE SCREEN.
 *
 * The counting is the adapter's and the pairing is the server's; what is pinned
 * here is the RENDERING. A skip the owner cannot see is worth exactly what no
 * count at all is worth — recoverable only by somebody with database access —
 * and that is the hole this file's subject closes.
 *
 * Four properties, asserted separately because they fail separately:
 *
 *   1. `left_out` says the number and the reason, on the row.
 *   2. `none` renders NOTHING, which keeps this column's standing invariant that
 *      no positive claim is ever painted on it.
 *   3. `none` and an ABSENT verdict are still different values, and the note
 *      above the table is where the difference becomes visible.
 *   4. The note draws the boundary: a skip count is not a completeness check.
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

describe('the skip verdict, parsed', () => {
	it('keeps the count and the reason under left_out', () => {
		expect(
			toSkips({ state: 'left_out', items: 42, reason: 'r', recorded_at: '2026-08-19T10:00:00Z' })
		).toEqual({ state: 'left_out', items: 42, reason: 'r', recordedAt: '2026-08-19T10:00:00Z' });
	});

	it('drops a count that arrives under `none`, because the label denies it', () => {
		expect(toSkips({ state: 'none', items: 9, reason: 'r' })).toEqual({ state: 'none' });
	});

	// ⚠️ NOT DEFAULTED TO `none`, WHICH WOULD BE THE COMFORTABLE GUESS AND THE
	// WRONG ONE. A member this build cannot read is a record it cannot render,
	// and the only honest rendering of that is no record.
	it('refuses a state no member matches rather than picking the nearest', () => {
		expect(toSkips({ state: 'mostly', items: 40 })).toBeUndefined();
		expect(toSkips({ state: '' })).toBeUndefined();
		expect(toSkips(undefined)).toBeUndefined();
	});

	it('leaves the key off the library when the server sent none', () => {
		expect(parse().skipped).toBeUndefined();
	});
});

describe('left_out says the number and the why, on the row', () => {
	it('names the count and carries the reason as the evidence', () => {
		const l = parse({
			skipped: {
				state: 'left_out',
				items: 42,
				reason: 'a file BookOrbit itself cannot classify has no row',
				recorded_at: '2026-08-19T10:24:00Z'
			}
		});
		expect(marks(l)).toContainEqual({
			word: 'Some items were left out',
			detail:
				'42 items were read and not mapped; a file BookOrbit itself cannot classify has no row'
		});
	});

	it('says `item was` for one', () => {
		const l = parse({ skipped: { state: 'left_out', items: 1, reason: 'r' } });
		expect(marks(l)[0].detail).toBe('1 item was read and not mapped; r');
	});

	// ⚠️ GREY, NOT AMBER, AND IT IS THE OPPOSITE CALL FROM THE SHORTFALL NEXT
	// DOOR. A content filter hiding books is a misconfiguration the owner can go
	// and fix. A skip is UsArr doing what it was built to do, so nothing is
	// broken and there is nothing to go and change — painting it amber would make
	// the shortfalls harder to find.
	it('is grey, because nothing is broken and there is nothing to go and fix', () => {
		const l = parse({ skipped: { state: 'left_out', items: 2, reason: 'r' } });
		const mark = libraryStates(l, HEALTH_UNREAD).find((m) => m.key === 'items-left-out');
		expect(mark?.tone).toBe('none');
	});

	it('carries the age, so the count reads as a measurement rather than a live fact', () => {
		const l = parse({
			skipped: { state: 'left_out', items: 2, reason: 'r', recorded_at: '2026-08-19T10:24:00Z' }
		});
		const mark = libraryStates(l, HEALTH_UNREAD).find((m) => m.key === 'items-left-out');
		expect(mark?.at).toBe('2026-08-19T10:24:00Z');
	});
});

describe('the two silences are one rendering and two values', () => {
	// The standing invariant: nothing on this column is ever a positive claim.
	// `none` is a real, measured negative and it still renders nothing, exactly
	// as `complete` does one axis over.
	it('renders nothing for `none`, and nothing for an absent verdict', () => {
		expect(marks(parse({ skipped: { state: 'none' } }))).toEqual([]);
		expect(marks(parse())).toEqual([]);
	});

	// ⚠️ AND THEY ARE STILL DIFFERENT VALUES, WHICH IS THE WHOLE POINT. If the
	// wire collapsed them, a library nothing had ever looked at would be
	// indistinguishable from one that was walked clean — and the note below could
	// not tell the user which rows its claim covers.
	it('keeps `none` and absent apart in the parsed library', () => {
		expect(parse({ skipped: { state: 'none' } }).skipped).toEqual({ state: 'none' });
		expect(parse().skipped).toBeUndefined();
	});
});

describe('the sentence above the table', () => {
	it('says nothing at all when nothing was left out', () => {
		expect(skippedNote([parse(), parse({ skipped: { state: 'none' } })])).toBe('');
	});

	// ⚠️ THE BOUNDARY IS THE HALF WORTH THE SENTENCE. One clean number must not
	// read as "this library is complete": that is a different measurement with a
	// different failure mode, and letting the two blur is the exact error the
	// completeness work exists to prevent.
	it('names the total and refuses to let it read as a completeness claim', () => {
		const note = skippedNote([parse({ skipped: { state: 'left_out', items: 42, reason: 'r' } })]);
		expect(note).toContain('42 items');
		expect(note).toContain('a library');
		expect(note).toContain('not a completeness check');
	});

	it('sums across libraries and counts them', () => {
		const note = skippedNote([
			parse({ skipped: { state: 'left_out', items: 40, reason: 'r' } }),
			parse({ skipped: { state: 'left_out', items: 2, reason: 'r' } })
		]);
		expect(note).toContain('42 items');
		expect(note).toContain('2 libraries');
	});

	// ⚠️ GATED ON THE FIRST ARM, DELIBERATELY. "N libraries are not counted this
	// way" is true of every row of every Kavita-only install, forever, which is
	// §17.4 rule 5's definition of noise. It becomes news the moment the screen
	// has told the user that items get left out, because then the reader needs to
	// know which rows the claim covers.
	it('bounds its own claim once it has made one, and not before', () => {
		expect(skippedNote([parse(), parse()])).toBe('');
		const note = skippedNote([
			parse({ skipped: { state: 'left_out', items: 3, reason: 'r' } }),
			parse(),
			parse()
		]);
		expect(note).toContain('2 other libraries');
		expect(note).toContain('nothing either way');
	});

	// A library that WAS observed and left nothing out is not "not counted": it
	// is counted, and the answer was zero. Folding it into the second arm would
	// understate what the screen knows.
	it('does not count a measured negative as uncounted', () => {
		const note = skippedNote([
			parse({ skipped: { state: 'left_out', items: 3, reason: 'r' } }),
			parse({ skipped: { state: 'none' } })
		]);
		expect(note).not.toContain('other librar');
	});
});
