/**
 * SORT STATE, AND ITS SYNCHRONISATION WITH THE URL.
 *
 * ⚠️ THIS IMPLEMENTS A SPECIFIED RULE, NOT AN INCIDENTAL ONE. The authority is
 * ADR-0038 — `ARCHITECTURE.md` §17.5 and `design/DESIGN-DIRECTION.md` §9.1a,
 * "a list freezes its order while a user is aiming at it", settled 2026-08-16
 * in four rounds across three threads. Clause 6 of that rule is the whole of
 * this file:
 *
 *   "The sort key and direction live in the URL — `&sort=` and `&dir=`,
 *    alongside the scope parameters — so a sorted view is linkable and survives
 *    a reload exactly rather than approximately."
 *
 * The freeze behaviour itself lives in `$lib/frozenorder.svelte`, which is the
 * other half of the same ADR. Neither is specific to the search route; both are
 * written to be attached to any results list that can change while a user is
 * looking at it.
 *
 * Everything here is DOM-free and pure so the node-environment vitest run can
 * exercise it.
 */

export type SortDir = 'asc' | 'desc';

/** A sort key and a direction. The pair is the unit; a key without a direction
 * is not a sort. */
export interface SortSpec {
	key: string;
	dir: SortDir;
}

/**
 * One column a list can be sorted by.
 *
 * `value` returns the number to compare, or `undefined` for "this row has no
 * value here". That is a real state rather than a zero — a usenet release
 * reports no seeders at all, and treating that as 0 would sort it below every
 * dead torrent instead of out of the comparison. See `compareBy`.
 */
export interface SortColumn<T> {
	/** Matches the `ListColumn.id` it sorts, and is what `&sort=` carries. */
	id: string;
	/**
	 * Which direction this key takes when the user selects it for the first
	 * time. More seeders and more grabs are better, so those start descending;
	 * a newer release is better, so age starts ascending. Making every column
	 * start ascending means the first click on `Seeders` shows the worst
	 * releases in the product, which is never what was asked for.
	 */
	defaultDir: SortDir;
	value: (row: T) => number | undefined;
}

/**
 * The comparator for one spec.
 *
 * ROWS WITH NO VALUE SORT LAST IN BOTH DIRECTIONS. Reversing the direction must
 * not promote "unknown" to the top of the list: a usenet row has no seeder
 * count, and putting the rows that cannot answer the question above the rows
 * that can is a worse answer than either ordering.
 *
 * THE TIE-BREAK IS THE ROW'S IDENTITY, and it is not optional. A comparator
 * that leaves ties unordered lets equal rows swap places on a re-sort, which is
 * precisely the movement under an engaged pointer that ADR-0038 exists to
 * prevent — and it would arrive from the sort itself rather than from new data,
 * so the "N new results" control would not even be showing.
 */
export function compareBy<T>(
	column: SortColumn<T> | undefined,
	dir: SortDir,
	key: (row: T) => string
): (a: T, b: T) => number {
	if (column === undefined) return (a, b) => key(a).localeCompare(key(b));
	const sign = dir === 'asc' ? 1 : -1;
	return (a, b) => {
		const av = column.value(a);
		const bv = column.value(b);
		if (av === undefined && bv === undefined) return key(a).localeCompare(key(b));
		if (av === undefined) return 1;
		if (bv === undefined) return -1;
		if (av !== bv) return (av - bv) * sign;
		return key(a).localeCompare(key(b));
	};
}

/** The direction a click on `id` should produce, given where the sort is now:
 * the same column toggles, a different column starts at its own default. */
export function nextDir<T>(
	columns: readonly SortColumn<T>[],
	current: SortSpec,
	id: string
): SortDir {
	if (current.key === id) return current.dir === 'asc' ? 'desc' : 'asc';
	return columns.find((c) => c.id === id)?.defaultDir ?? 'asc';
}

/**
 * Read `&sort=` and `&dir=` back out of a URL.
 *
 * An unknown key falls back to the default rather than erroring: a link that
 * names a column this build no longer has should still open the screen, and it
 * is the screen's own sort control that then says what it is actually sorted
 * by.
 */
export function readSort<T>(
	params: URLSearchParams,
	columns: readonly SortColumn<T>[],
	fallback: SortSpec
): SortSpec {
	const key = params.get('sort') ?? '';
	const column = columns.find((c) => c.id === key);
	if (column === undefined) return fallback;
	const dir = params.get('dir');
	return { key: column.id, dir: dir === 'asc' || dir === 'desc' ? dir : column.defaultDir };
}

/** Write a spec into a `URLSearchParams`, mutating it in place. */
export function writeSort(params: URLSearchParams, spec: SortSpec): void {
	params.set('sort', spec.key);
	params.set('dir', spec.dir);
}

/**
 * `aria-sort` for a column header. `none` rather than the attribute's absence,
 * because a sortable column that is not the active one is a different fact from
 * a column that cannot be sorted at all, and only the first gets the attribute.
 */
export function ariaSort(spec: SortSpec, id: string): 'ascending' | 'descending' | 'none' {
	if (spec.key !== id) return 'none';
	return spec.dir === 'asc' ? 'ascending' : 'descending';
}
