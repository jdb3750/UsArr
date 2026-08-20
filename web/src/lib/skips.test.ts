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

	/* ── the shared container, on the row ──────────────────────────────────────
	 *
	 * ⚠️ THE COUNT IS NOT SPLIT AND NOT APPORTIONED. A skip is a fact about the
	 * upstream container, and since ADR-0066 decision 5 a `book` library and a
	 * `comic` library can stand over one. Both rows fold the same `sync_report`
	 * row and both are TRUE. What would be false is the reader adding them, and
	 * §17.8 has no container-level slot to state the fact in once instead — so
	 * the row says it.
	 */

	const fiction = () =>
		parse({ id: 2, name: 'Fiction', skipped: { state: 'left_out', items: 1, reason: 'r' } });
	const comics = () =>
		parse({
			id: 3,
			name: 'Fiction (Comics)',
			kind: 'comic',
			skipped: { state: 'left_out', items: 1, reason: 'r' }
		});

	it('names the sibling that reports the same skip over the same container', () => {
		const rows = [fiction(), comics()];
		const mark = libraryStates(rows[1], HEALTH_UNREAD, rows).find(
			(m) => m.key === 'items-left-out'
		);
		expect(mark?.detail).toBe(
			'1 item was read and not mapped; r; the same skip is reported by Fiction over the same upstream container, so it is one skip shown on both rows'
		);
	});

	it('says it on BOTH rows, because neither of them owns the skip', () => {
		const rows = [fiction(), comics()];
		const mark = libraryStates(rows[0], HEALTH_UNREAD, rows).find(
			(m) => m.key === 'items-left-out'
		);
		expect(mark?.detail).toContain('the same skip is reported by Fiction (Comics)');
	});

	it('keeps the count whole on each row rather than halving it between them', () => {
		const rows = [fiction(), comics()];
		for (const r of rows) {
			const mark = libraryStates(r, HEALTH_UNREAD, rows).find((m) => m.key === 'items-left-out');
			expect(mark?.detail).toContain('1 item was read and not mapped');
			expect(mark?.detail).not.toContain('0.5');
		}
	});

	it('says nothing about a sibling when no other row reads the same container', () => {
		const other = parse({
			id: 3,
			name: 'Other',
			sources: [{ ...(wire().sources as Record<string, unknown>[])[0], container_ref: '13' }],
			skipped: { state: 'left_out', items: 5, reason: 'r' }
		});
		const rows = [fiction(), other];
		const mark = libraryStates(rows[0], HEALTH_UNREAD, rows).find(
			(m) => m.key === 'items-left-out'
		);
		expect(mark?.detail).toBe('1 item was read and not mapped; r');
	});

	it('omits the clause when the caller passes no list, and changes nothing else', () => {
		const mark = libraryStates(fiction(), HEALTH_UNREAD).find((m) => m.key === 'items-left-out');
		expect(mark?.detail).toBe('1 item was read and not mapped; r');
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
		// ⚠️ TWO DIFFERENT CONTAINERS, AND THE `container_ref` IS THE POINT. The
		// base fixture is one BookOrbit library, so two copies of it are two rows
		// over ONE container — which is the shared-skip case below, not this one.
		// A summing test built from the same container would assert that a
		// double-count happens.
		const note = skippedNote([
			parse({ id: 2, skipped: { state: 'left_out', items: 40, reason: 'r' } }),
			parse({
				id: 3,
				sources: [{ ...(wire().sources as Record<string, unknown>[])[0], container_ref: '13' }],
				skipped: { state: 'left_out', items: 2, reason: 'r' }
			})
		]);
		expect(note).toContain('42 items');
		expect(note).toContain('2 libraries');
	});

	/* ── the shared container: one skip, two rows ──────────────────────────────
	 *
	 * ADR-0066 decision 5 puts a `book` library and a `comic` library over ONE
	 * upstream container, and the server folds that container's `sync_report`
	 * rows into BOTH of them. The skip happened to the container, once. Neither
	 * sibling owns it, so it is never split and never apportioned — and the
	 * screen must not let the reader add it to itself.
	 */

	it('counts a container once when two libraries stand over it', () => {
		const note = skippedNote([
			parse({ id: 2, name: 'Fiction', skipped: { state: 'left_out', items: 1, reason: 'r' } }),
			parse({
				id: 3,
				name: 'Fiction (Comics)',
				kind: 'comic',
				skipped: { state: 'left_out', items: 1, reason: 'r' }
			})
		]);
		expect(note).toContain('1 item');
		expect(note).not.toContain('2 items');
		expect(note).toContain('the same skip is reported on each of them and is counted once here');
	});

	/* ── two instances, two mixed containers: the shape the row total cannot ──
	 * dedupe on its own.
	 *
	 * ⚠️ THIS IS WHY THE BREAKDOWN IS ON THE WIRE AT ALL. Two BookOrbit
	 * instances, each holding one mixed container, and §17.8 binds:
	 *
	 *     Fiction          book   over A/1 and B/9
	 *     Fiction (Comics) comic  over A/1
	 *     Fiction (2)      comic  over B/9
	 *
	 * A/1 left one item out and B/9 left two out, so the truth is THREE. The
	 * three rows report 3, 1 and 2, and their container SIGNATURES are all
	 * different, so a fold on the signature collapses nothing and reports six.
	 *
	 * ⚠️ AND NO ARITHMETIC ON THE ROW TOTALS RECOVERS IT. 3, 1 and 2 are each
	 * true of their own row; which container each part happened in is simply not
	 * in them. That is MISSING DATA rather than a client bug, so the fix is the
	 * server sending the per-container breakdown — apportioning a row's total
	 * across its signatures would invent a precision nobody measured, and a skip
	 * is a fact about a container rather than about a library.
	 */

	const src = (instance: number, ref: string) => ({
		id: instance * 100 + Number(ref),
		service_instance_id: instance,
		service_name: instance === 1 ? 'BookOrbit A' : 'BookOrbit B',
		service_kind: 'bookorbit',
		container_kind: 'remote_library',
		container_ref: ref,
		is_metadata_authority: instance === 1
	});
	const part = (instance: number, ref: string, items: number) => ({
		service_instance_id: instance,
		container_kind: 'remote_library',
		container_ref: ref,
		items
	});
	const twoInstances = () => [
		parse({
			id: 2,
			name: 'Fiction',
			sources: [src(1, '1'), src(2, '9')],
			skipped: {
				state: 'left_out',
				items: 3,
				reason: 'r',
				containers: [part(1, '1', 1), part(2, '9', 2)]
			}
		}),
		parse({
			id: 3,
			name: 'Fiction (Comics)',
			kind: 'comic',
			sources: [src(1, '1')],
			skipped: { state: 'left_out', items: 1, reason: 'r', containers: [part(1, '1', 1)] }
		}),
		parse({
			id: 4,
			name: 'Fiction (2)',
			kind: 'comic',
			sources: [src(2, '9')],
			skipped: { state: 'left_out', items: 2, reason: 'r', containers: [part(2, '9', 2)] }
		})
	];

	it('counts each container once when the rows overlap only in part', () => {
		const note = skippedNote(twoInstances());
		expect(note).toContain('3 items');
		expect(note).not.toContain('6 items');
	});

	it('names both shared containers rather than the rows that share them', () => {
		const note = skippedNote(twoInstances());
		expect(note).toContain(
			'2 upstream containers are each reported by more than one row, so the same skip is reported on each of them and is counted once here.'
		);
	});

	it('keeps every row whole: each row total is its own containers summed', () => {
		for (const l of twoInstances()) {
			const parts = l.skipped?.containers ?? [];
			expect(parts.reduce((n, c) => n + c.items, 0)).toBe(l.skipped?.items);
		}
	});

	it('says nothing about sharing when every row stands over its own container', () => {
		const note = skippedNote([
			parse({ id: 2, skipped: { state: 'left_out', items: 40, reason: 'r' } }),
			parse({
				id: 3,
				sources: [{ ...(wire().sources as Record<string, unknown>[])[0], container_ref: '13' }],
				skipped: { state: 'left_out', items: 2, reason: 'r' }
			})
		]);
		expect(note).not.toContain('same skip');
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
