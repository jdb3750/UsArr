import { describe, expect, it } from 'vitest';
import { ariaSort, compareBy, nextDir, readSort, writeSort, type SortColumn } from './sortspec';

interface Row {
	id: string;
	seeders?: number;
}

const seeders: SortColumn<Row> = { id: 'peers', defaultDir: 'desc', value: (r) => r.seeders };
const age: SortColumn<Row> = { id: 'age', defaultDir: 'asc', value: () => 1 };
const COLUMNS = [seeders, age];
const key = (r: Row) => r.id;

describe('compareBy', () => {
	it('orders ascending and descending by the column value', () => {
		const rows: Row[] = [
			{ id: 'a', seeders: 5 },
			{ id: 'b', seeders: 1 },
			{ id: 'c', seeders: 9 }
		];
		expect([...rows].sort(compareBy(seeders, 'asc', key)).map(key)).toEqual(['b', 'a', 'c']);
		expect([...rows].sort(compareBy(seeders, 'desc', key)).map(key)).toEqual(['c', 'a', 'b']);
	});

	// A usenet release reports no seeders at all. Reversing the direction must not
	// promote the rows that cannot answer the question above the rows that can.
	it('sorts rows with no value last in BOTH directions', () => {
		const rows: Row[] = [{ id: 'a' }, { id: 'b', seeders: 3 }, { id: 'c', seeders: 7 }];
		expect([...rows].sort(compareBy(seeders, 'asc', key)).map(key)).toEqual(['b', 'c', 'a']);
		expect([...rows].sort(compareBy(seeders, 'desc', key)).map(key)).toEqual(['c', 'b', 'a']);
	});

	// ADR-0038: an unordered tie lets equal rows swap places on a re-sort, which
	// is movement under an engaged pointer arriving from the sort itself.
	it('breaks ties on identity, so equal rows have one order', () => {
		const rows: Row[] = [
			{ id: 'c', seeders: 4 },
			{ id: 'a', seeders: 4 },
			{ id: 'b', seeders: 4 }
		];
		const once = [...rows].sort(compareBy(seeders, 'desc', key)).map(key);
		const twice = [...rows]
			.reverse()
			.sort(compareBy(seeders, 'desc', key))
			.map(key);
		expect(once).toEqual(['a', 'b', 'c']);
		expect(twice).toEqual(once);
	});

	it('falls back to identity order when the column is unknown', () => {
		const rows: Row[] = [{ id: 'b' }, { id: 'a' }];
		expect([...rows].sort(compareBy(undefined, 'asc', key)).map(key)).toEqual(['a', 'b']);
	});
});

describe('nextDir', () => {
	it('toggles the active column and starts a new one at its own default', () => {
		expect(nextDir(COLUMNS, { key: 'peers', dir: 'desc' }, 'peers')).toBe('asc');
		expect(nextDir(COLUMNS, { key: 'peers', dir: 'asc' }, 'peers')).toBe('desc');
		expect(nextDir(COLUMNS, { key: 'age', dir: 'asc' }, 'peers')).toBe('desc');
		expect(nextDir(COLUMNS, { key: 'peers', dir: 'desc' }, 'age')).toBe('asc');
	});

	it('does not throw on a column it has never heard of', () => {
		expect(nextDir(COLUMNS, { key: 'age', dir: 'asc' }, 'nope')).toBe('asc');
	});
});

describe('readSort and writeSort', () => {
	const fallback = { key: 'age', dir: 'asc' } as const;

	it('round-trips a spec through a URL', () => {
		const params = new URLSearchParams();
		writeSort(params, { key: 'peers', dir: 'desc' });
		expect(params.toString()).toBe('sort=peers&dir=desc');
		expect(readSort(params, COLUMNS, fallback)).toEqual({ key: 'peers', dir: 'desc' });
	});

	it('falls back rather than erroring on a key this build does not have', () => {
		const params = new URLSearchParams('sort=gone&dir=desc');
		expect(readSort(params, COLUMNS, fallback)).toEqual(fallback);
	});

	it('takes the column default when the direction is missing or junk', () => {
		expect(readSort(new URLSearchParams('sort=peers'), COLUMNS, fallback)).toEqual({
			key: 'peers',
			dir: 'desc'
		});
		expect(readSort(new URLSearchParams('sort=peers&dir=sideways'), COLUMNS, fallback)).toEqual({
			key: 'peers',
			dir: 'desc'
		});
	});
});

describe('ariaSort', () => {
	it('distinguishes "sortable but not active" from "not sortable"', () => {
		expect(ariaSort({ key: 'peers', dir: 'asc' }, 'peers')).toBe('ascending');
		expect(ariaSort({ key: 'peers', dir: 'desc' }, 'peers')).toBe('descending');
		expect(ariaSort({ key: 'peers', dir: 'desc' }, 'age')).toBe('none');
	});
});
