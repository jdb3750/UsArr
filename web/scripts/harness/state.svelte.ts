/**
 * The harness's fabricated corpus and the handle the Playwright driver pokes.
 *
 * FABRICATED DATA, DEV ONLY. Never imported by anything under src/routes.
 */
import type { ListState } from '../../src/lib/list';

/**
 * ⚠️ THE WIRE SHAPE, NOT PRE-FORMATTED TEXT — AND FOR `sizeBytes` AND `ageDays`
 * THAT IS THE WHOLE POINT RATHER THAN A STYLE PREFERENCE.
 *
 * `internal/httpapi/search.go` sends `size_bytes` as an int64 count of BYTES and
 * `age_days` as a float; `$lib/format` is the only thing that turns either into
 * text, and it prints BINARY units (`B`/`KiB`/`MiB`/`GiB`/`TiB`) and an age in
 * `h` or `d`. This corpus used to carry `size: "12.34 GB"` and `age: "137 days"`
 * — a DECIMAL unit family the product cannot emit, pinned to one unit for every
 * row, beside a word no formatter here produces. Both are now the numbers the
 * server sends, rendered through the app's own formatter in `Harness.svelte`.
 *
 * That is not tidiness. A width measured against a family the product never
 * emits comes back confidently wrong — it is how a unit reserve on this same
 * table was sized 0.5ch too narrow against the design mockups' `MB`/`GB`, and a
 * corpus that never leaves one decade cannot render the widest member of the
 * family it is reserving for.
 */
export interface Release {
	id: string;
	release: string;
	indexer: string;
	/** `size_bytes` — bytes, as the wire sends them. */
	sizeBytes: number;
	/** `age_days` — a float, as the wire sends it. */
	ageDays: number;
	/**
	 * ⚠️ REQUIRED HERE, `*int32` + `omitempty` UPSTREAM, AND THE DIVERGENCE IS
	 * DELIBERATE. A usenet release carries NEITHER count, and the real table
	 * renders `Not applicable` for it — which is why `routes/requests` gives
	 * Peers a measured 112px track after that string wrapped at 88px. This
	 * harness's Peers track is `8ch`, so it could not render that case
	 * faithfully whatever the data said; and a usenet arm would also have to
	 * carry no flags (`GetFlags` runs only for torrents), which would change the
	 * chip distribution that IS what sets this row's height. Modelling it here
	 * would move the published mean without buying a real measurement. See the
	 * column-set note in `Harness.svelte`.
	 */
	seeders: number;
	leechers: number;
	flags: string[];
}

const INDEXERS = ['TorrentLeech', 'IPTorrents', 'AlphaRatio', 'Nyaa', 'BroadcastTheNet'];
const GROUPS = ['NTb', 'FLUX', 'CtrlHD', 'SMURF', 'TEPES', 'playWEB'];
const CODECS = ['x264', 'x265', 'H.264', 'AV1'];
const FLAGS = ['Freeleech', 'Internal', 'Scene', 'Golden', 'Double upload'];

/**
 * Deterministic: a benchmark whose input changes between runs cannot be
 * compared with itself. A 32-bit LCG is enough and needs no dependency.
 *
 * ⚠️ EIGHT DRAWS PER ROW, AND THE COUNT IS LOAD-BEARING. `flagCount` is drawn
 * THIRD, and the number of chips in the flags cell is the only thing in this row
 * that changes its height — the measured bimodality is 44px×1308 / 48px×692 at
 * compact, and it is the chip cell wrapping, not the controls. Spend one extra
 * `next()` anywhere and every later row draws a different `flagCount`, which
 * moves the mean this harness exists to produce and moves `ROW_INTRINSIC` with
 * it, for no reason a reader could ever find. So `sizeBytes` takes ONE draw and
 * slices it, rather than taking two.
 */
function makeRows(count: number, seed = 1): Release[] {
	let s = seed >>> 0;
	const next = () => (s = (s * 1664525 + 1013904223) >>> 0);
	const rows: Release[] = new Array(count);
	for (let i = 0; i < count; i++) {
		const g = GROUPS[next() % GROUPS.length];
		const c = CODECS[next() % CODECS.length];
		const flagCount = next() % 6;
		const indexer = INDEXERS[next() % INDEXERS.length];
		const size = next();
		rows[i] = {
			id: `rel-${i}`,
			release: `Some.Long.Release.Name.S0${1 + (i % 9)}E${String(1 + (i % 24)).padStart(2, '0')}.2160p.WEB-DL.DDP5.1.${c}-${g}`,
			indexer,
			// Spans EVERY unit `sizeParts` can print, B through TiB, rather than
			// sitting in one decade: the magnitude comes off the draw's HIGH bits,
			// which an LCG distributes well, and the mantissa off its low ones. A
			// corpus that is always ~1 GiB never renders the widest unit, which is
			// exactly how a too-narrow reserve survives a green run.
			sizeBytes: Math.round((1 + (size % 102300) / 100) * 1024 ** ((size >>> 28) % 5)),
			seeders: next() % 900,
			leechers: next() % 90,
			// Deliberately crosses 1.0, so `formatAge`'s hours branch ("13 h") is
			// rendered here rather than assumed. The old corpus started at 1 day.
			ageDays: (next() % 40000) / 100,
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
