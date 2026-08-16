/**
 * The harness's fabricated corpus and the handle the Playwright driver pokes.
 *
 * FABRICATED DATA, DEV ONLY. Never imported by anything under src/routes.
 */
import type { ListState } from '../../src/lib/list';

export interface Release {
	id: string;
	release: string;
	indexer: string;
	size: string;
	seeders: number;
	leechers: number;
	age: string;
	flags: string[];
}

const INDEXERS = ['TorrentLeech', 'IPTorrents', 'AlphaRatio', 'Nyaa', 'BroadcastTheNet'];
const GROUPS = ['NTb', 'FLUX', 'CtrlHD', 'SMURF', 'TEPES', 'playWEB'];
const CODECS = ['x264', 'x265', 'H.264', 'AV1'];
const FLAGS = ['Freeleech', 'Internal', 'Scene', 'Golden', 'Double upload'];

/**
 * Deterministic: a benchmark whose input changes between runs cannot be
 * compared with itself. A 32-bit LCG is enough and needs no dependency.
 */
function makeRows(count: number, seed = 1): Release[] {
	let s = seed >>> 0;
	const next = () => (s = (s * 1664525 + 1013904223) >>> 0);
	const rows: Release[] = new Array(count);
	for (let i = 0; i < count; i++) {
		const g = GROUPS[next() % GROUPS.length];
		const c = CODECS[next() % CODECS.length];
		const flagCount = next() % 6;
		rows[i] = {
			id: `rel-${i}`,
			release: `Some.Long.Release.Name.S0${1 + (i % 9)}E${String(1 + (i % 24)).padStart(2, '0')}.2160p.WEB-DL.DDP5.1.${c}-${g}`,
			indexer: INDEXERS[next() % INDEXERS.length],
			size: `${(1 + (next() % 9000) / 100).toFixed(2)} GB`,
			seeders: next() % 900,
			leechers: next() % 90,
			age: `${1 + (next() % 400)} days`,
			flags: FLAGS.slice(0, flagCount)
		};
	}
	return rows;
}

let rows = $state<Release[]>(makeRows(200));
let total = $state<number | undefined>(1204);
let state = $state<ListState>('default');
let hasMore = $state(true);
let stack = $state<'labels' | 'two-line'>('two-line');
/**
 * `simple` strips the row down to one line of text per cell — no chips, no
 * buttons, no `<select>`. It exists for exactly one measurement: the
 * contain-intrinsic-size DEFAULT in src/lib/list.ts is documented as the
 * content-box height of a ONE-LINE row, and this list's real rows are not one
 * line (a 32px `<select>` alone puts the floor above every density's --row-h).
 * Measuring the rich row and calling the answer "one line" would be the same
 * class of mistake as deriving the placeholder from --row-h.
 */
let simple = $state(false);
/** Passed straight through to List's `rowIntrinsic` prop. */
let rowIntrinsic = $state<number | undefined>(undefined);
/** Bumped to force every row element to be destroyed and rebuilt. */
let generation = $state(0);

export const harness = {
	get rows() {
		return rows;
	},
	get total() {
		return total;
	},
	get state() {
		return state;
	},
	get hasMore() {
		return hasMore;
	},
	get stack() {
		return stack;
	},
	get simple() {
		return simple;
	},
	setSimple(next: boolean) {
		simple = next;
	},
	get rowIntrinsic() {
		return rowIntrinsic;
	},
	setRowIntrinsic(next: number | undefined) {
		rowIntrinsic = next;
	},

	/**
	 * Destroy every row element and build fresh ones.
	 *
	 * `contain-intrinsic-size: auto` REMEMBERS the last size an element actually
	 * rendered at, and it keeps that memory when the element goes off screen
	 * again. So a keyed {#each} that reuses the same DOM nodes across a change
	 * of density carries the OLD density's remembered heights, and any drift
	 * measurement taken over reused nodes measures the previous run rather than
	 * the value under test. Changing the key namespace is what forces new
	 * elements. (This is also a real product behaviour, not only a harness
	 * concern — see the density-change drift measurement in list-bench.mjs.)
	 */
	regenerate() {
		generation += 1;
		const count = rows.length;
		rows = makeRows(count, 1).map((r, i) => ({ ...r, id: `g${generation}-rel-${i}` }));
	},

	/** Replace the corpus wholesale. */
	setRows(count: number, seed = 1) {
		rows = makeRows(count, seed).map((r, i) => ({ ...r, id: `g${generation}-rel-${i}` }));
		total = Math.max(count, 1204);
	},

	/** The real "Load more" path: append, never replace. */
	loadMore(count = 200) {
		const start = rows.length;
		const extra = makeRows(count, 7).map((r, i) => ({
			...r,
			id: `g${generation}-rel-${start + i}`
		}));
		rows = [...rows, ...extra];
	},

	/**
	 * Reverse the corpus in place of identity. This is the test that a
	 * positional key would fail: every row moves, and the roved row must stay
	 * with its own data rather than with its old index.
	 */
	reverse() {
		rows = [...rows].reverse();
	},

	setState(next: ListState) {
		state = next;
	},
	setTotal(next: number | undefined) {
		total = next;
	},
	setHasMore(next: boolean) {
		hasMore = next;
	},
	setStack(next: 'labels' | 'two-line') {
		stack = next;
	}
};
