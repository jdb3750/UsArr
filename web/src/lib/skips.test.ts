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

/**
 * The `containers` breakdown the server sends beside EVERY `left_out`, built
 * here from the row's own source so no fixture models a wire that cannot occur.
 *
 * ⚠️ A `left_out` VERDICT WITHOUT IT IS NOT A SHAPE THIS SCREEN CAN MEET. The
 * SPA is embedded in the binary that serves it, so a build whose `skippedNote`
 * reads `containers` is a build whose handler writes it — there is no fallback
 * in `skipParts` and no fixture here may pretend there is one.
 */
function over(items: number, ref = '12', instance = 1): Record<string, unknown> {
	return {
		service_instance_id: instance,
		container_kind: 'remote_library',
		container_ref: ref,
		items
	};
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

	it('keeps the per-container breakdown, which is what makes a skip dedupable', () => {
		expect(
			toSkips({
				state: 'left_out',
				items: 3,
				containers: [over(1, '1', 1), over(2, '9', 2)]
			})?.containers
		).toEqual([
			{ serviceInstanceId: 1, containerKind: 'remote_library', containerRef: '1', items: 1 },
			{ serviceInstanceId: 2, containerKind: 'remote_library', containerRef: '9', items: 2 }
		]);
	});

	// ⚠️ ENTRY BY ENTRY, WHICH IS THE ONE PLACE THIS MODULE'S "DROP THE WHOLE
	// RECORD" REFLEX WOULD BE WRONG: each entry answers the dedupe question for
	// itself, so binning the good ones with the bad takes away the answer for the
	// containers that were fine.
	it('drops a malformed or zero entry and keeps the rest', () => {
		expect(
			toSkips({
				state: 'left_out',
				items: 3,
				containers: [over(1, '1', 1), 'not an object', over(0, '5', 1), { items: 4 }]
			})?.containers
		).toEqual([
			{ serviceInstanceId: 1, containerKind: 'remote_library', containerRef: '1', items: 1 }
		]);
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
				recorded_at: '2026-08-19T10:24:00Z',
				containers: [over(42)]
			}
		});
		expect(marks(l)).toContainEqual({
			word: 'Some items were left out',
			detail:
				'42 items were read and not mapped; a file BookOrbit itself cannot classify has no row'
		});
	});

	it('says `item was` for one', () => {
		const l = parse({
			skipped: { state: 'left_out', items: 1, reason: 'r', containers: [over(1)] }
		});
		expect(marks(l)[0].detail).toBe('1 item was read and not mapped; r');
	});

	// ⚠️ GREY, NOT AMBER, AND IT IS THE OPPOSITE CALL FROM THE SHORTFALL NEXT
	// DOOR. A content filter hiding books is a misconfiguration the owner can go
	// and fix. A skip is UsArr doing what it was built to do, so nothing is
	// broken and there is nothing to go and change — painting it amber would make
	// the shortfalls harder to find.
	it('is grey, because nothing is broken and there is nothing to go and fix', () => {
		const l = parse({
			skipped: { state: 'left_out', items: 2, reason: 'r', containers: [over(2)] }
		});
		const mark = libraryStates(l, HEALTH_UNREAD).find((m) => m.key === 'items-left-out');
		expect(mark?.tone).toBe('none');
	});

	it('carries the age, so the count reads as a measurement rather than a live fact', () => {
		const l = parse({
			skipped: {
				state: 'left_out',
				items: 2,
				reason: 'r',
				recorded_at: '2026-08-19T10:24:00Z',
				containers: [over(2)]
			}
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
		parse({
			id: 2,
			name: 'Fiction',
			skipped: { state: 'left_out', items: 1, reason: 'r', containers: [over(1)] }
		});
	const comics = () =>
		parse({
			id: 3,
			name: 'Fiction (Comics)',
			kind: 'comic',
			skipped: { state: 'left_out', items: 1, reason: 'r', containers: [over(1)] }
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
			skipped: { state: 'left_out', items: 5, reason: 'r', containers: [over(5, '13')] }
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

	/* ── the PARTIAL overlap, which is the shape the clause above was written ──
	 * for and the shape it did not cover.
	 *
	 * ⚠️ THE SIGNATURES DIFFER WHILE THE SKIP IS SHARED, and that is the whole
	 * case. Two containers, P and Q; Fiction stands over both and Comics over P
	 * alone. P's skip is folded into BOTH rows, so the reader sees it twice —
	 * exactly the double-count the clause exists to join — while no comparison
	 * of the rows' container SETS ever finds them equal. Keying the clause on
	 * the library's set therefore leaves BOTH rows silent about the one thing
	 * that would have stopped the addition.
	 */
	const partial = () => {
		const p = {
			id: 101,
			service_instance_id: 1,
			service_name: 'BookOrbit A',
			service_kind: 'bookorbit',
			container_kind: 'remote_library',
			container_ref: '1',
			is_metadata_authority: true
		};
		const q = {
			...p,
			id: 209,
			service_instance_id: 2,
			service_name: 'BookOrbit B',
			container_ref: '9'
		};
		return [
			parse({
				id: 2,
				name: 'Fiction',
				sources: [p, q],
				skipped: {
					state: 'left_out',
					items: 3,
					reason: 'r',
					containers: [over(1, '1', 1), over(2, '9', 2)]
				}
			}),
			parse({
				id: 3,
				name: 'Fiction (Comics)',
				kind: 'comic',
				sources: [p],
				skipped: { state: 'left_out', items: 1, reason: 'r', containers: [over(1, '1', 1)] }
			})
		];
	};

	it('names the sharing on both rows when the rows overlap only in part', () => {
		const rows = partial();
		for (const r of rows) {
			const mark = libraryStates(r, HEALTH_UNREAD, rows).find((m) => m.key === 'items-left-out');
			expect(mark?.detail).toContain('upstream container');
		}
	});

	it('names the OTHER row, so the reader can see which two numbers overlap', () => {
		const rows = partial();
		const a = libraryStates(rows[0], HEALTH_UNREAD, rows).find((m) => m.key === 'items-left-out');
		const b = libraryStates(rows[1], HEALTH_UNREAD, rows).find((m) => m.key === 'items-left-out');
		expect(a?.detail).toContain('Fiction (Comics)');
		expect(b?.detail).toContain('Fiction');
	});

	// ⚠️ AND THE TWO ROWS DO NOT SAY THE SAME THING, BECAUSE THE TWO FACTS ARE
	// NOT THE SAME FACT. Only 1 of Fiction's 3 is shared, so a clause telling its
	// reader the counts are one and the same would swap one wrong number for
	// another; all of Comics' 1 is shared, so for that row they ARE one number.
	it('says how much of each row is shared, rather than one sentence for both', () => {
		const rows = partial();
		const a = libraryStates(rows[0], HEALTH_UNREAD, rows).find((m) => m.key === 'items-left-out');
		const b = libraryStates(rows[1], HEALTH_UNREAD, rows).find((m) => m.key === 'items-left-out');
		expect(a?.detail).toBe(
			'3 items were read and not mapped; r; part of this count is also reported by ' +
				'Fiction (Comics), which stands over an upstream container this library also ' +
				'reports, so those items are one skip shown on both rows'
		);
		expect(b?.detail).toBe(
			'1 item was read and not mapped; r; the same skip is reported by Fiction over the ' +
				'same upstream container, so it is one skip shown on both rows'
		);
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
		const note = skippedNote([
			parse({ skipped: { state: 'left_out', items: 42, reason: 'r', containers: [over(42)] } })
		]);
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
			parse({
				id: 2,
				skipped: { state: 'left_out', items: 40, reason: 'r', containers: [over(40)] }
			}),
			parse({
				id: 3,
				sources: [{ ...(wire().sources as Record<string, unknown>[])[0], container_ref: '13' }],
				skipped: { state: 'left_out', items: 2, reason: 'r', containers: [over(2, '13')] }
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
			parse({
				id: 2,
				name: 'Fiction',
				skipped: { state: 'left_out', items: 1, reason: 'r', containers: [over(1)] }
			}),
			parse({
				id: 3,
				name: 'Fiction (Comics)',
				kind: 'comic',
				skipped: { state: 'left_out', items: 1, reason: 'r', containers: [over(1)] }
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
	const twoInstances = () => [
		parse({
			id: 2,
			name: 'Fiction',
			sources: [src(1, '1'), src(2, '9')],
			skipped: {
				state: 'left_out',
				items: 3,
				reason: 'r',
				containers: [over(1, '1', 1), over(2, '9', 2)]
			}
		}),
		parse({
			id: 3,
			name: 'Fiction (Comics)',
			kind: 'comic',
			sources: [src(1, '1')],
			skipped: { state: 'left_out', items: 1, reason: 'r', containers: [over(1, '1', 1)] }
		}),
		parse({
			id: 4,
			name: 'Fiction (2)',
			kind: 'comic',
			sources: [src(2, '9')],
			skipped: { state: 'left_out', items: 2, reason: 'r', containers: [over(2, '9', 2)] }
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
			parse({
				id: 2,
				skipped: { state: 'left_out', items: 40, reason: 'r', containers: [over(40)] }
			}),
			parse({
				id: 3,
				sources: [{ ...(wire().sources as Record<string, unknown>[])[0], container_ref: '13' }],
				skipped: { state: 'left_out', items: 2, reason: 'r', containers: [over(2, '13')] }
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
			parse({ skipped: { state: 'left_out', items: 3, reason: 'r', containers: [over(3)] } }),
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
			parse({ skipped: { state: 'left_out', items: 3, reason: 'r', containers: [over(3)] } }),
			parse({ skipped: { state: 'none' } })
		]);
		expect(note).not.toContain('other librar');
	});
});
