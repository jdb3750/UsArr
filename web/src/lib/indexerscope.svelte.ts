/**
 * WHICH INDEXERS A SEARCH IS ALLOWED TO ASK.
 *
 * The server already accepts `?indexer=` repeated and narrows the fan-out
 * BEFORE the legs are planned (internal/httpapi/search.go → releases.Query), so
 * an unselected indexer is never asked and spends none of its rate limit. The
 * client was sending only `?query=`, which is why a Prowlarr carrying a dozen
 * indexers answered every search with everything.
 *
 * ⚠️ WHERE THE LIST OF INDEXERS COMES FROM, AND WHAT IT IS NOT.
 *
 * THERE IS NO ENDPOINT THAT LISTS A PROWLARR INSTANCE'S INDEXERS. As of this
 * commit `internal/httpapi/server.go` registers services, search, grab, events,
 * auth and health, and nothing that enumerates indexers. So this catalogue is
 * built from what searches have ALREADY REPORTED, and the screen says so rather
 * than implying it is Prowlarr's configured list.
 *
 * That fallback is better than it sounds, and worse than an endpoint:
 *
 *   better  every `search.indexer` frame names an indexer as the fan-out
 *           STARTS it, and the closing `search.done` report names every
 *           indexer including the ones that were skipped or blocked. So one
 *           unscoped search populates the full set for that instance, not just
 *           the indexers that happened to return a row.
 *   worse   before the first search there is nothing to pick from, and after a
 *           SCOPED search only the selected indexers report. The catalogue is
 *           therefore a UNION that only ever grows, held in localStorage so it
 *           survives a reload, and it is never narrowed by a scoped search.
 *
 * STORAGE KEYS ARE PART OF THE CONTRACT, exactly as in `$lib/prefs.svelte`:
 * `usarr.search.indexers` and `usarr.search.known-indexers` are what a browser
 * that has already been used will have written. Renaming one silently resets
 * every existing install, so they are frozen. Reads and writes both go through
 * try/catch — localStorage throws rather than returning null in a browser with
 * site data blocked, and a filter is never worth taking the screen down for.
 */
import { browser } from '$app/environment';

export const SELECTION_KEY = 'usarr.search.indexers';
export const KNOWN_KEY = 'usarr.search.known-indexers';

/**
 * ⚠️ THE PERSISTENCE SWITCH. THIS IS THE ONE LINE.
 *
 * Whether the indexer selection survives from one search to the next.
 *
 *   true   sticky. The selection is remembered across searches and across
 *          reloads, so someone who only ever wants one tracker sets it once.
 *   false  per-search. Every new query starts at "all indexers".
 *
 * SHIPPED VALUE: true. Joe asked for sticky in as many words — "i like sticky"
 * — after using the product, so this is the decision rather than a provisional
 * default. It stays a named constant because the behaviour is cheap to reverse
 * and expensive to find if it is scattered: flipping this one line is the whole
 * change, and nothing else in the codebase assumes either answer.
 *
 * ⚠️ STICKY MUST BE VISIBLE, AND THAT IS NOT OPTIONAL. A filter that silently
 * persists across sessions is how somebody concludes an indexer is broken when
 * they simply left it deselected weeks ago. Whenever the scope is anything
 * other than "all indexers", the screen states it in words next to the search
 * box — see `summary` below and its caller.
 */
export const SELECTION_PERSISTS = true;

/** One indexer, as a search has reported it. */
export interface KnownIndexer {
	id: number;
	name: string;
}

function read(key: string): string | null {
	if (!browser) return null;
	try {
		return window.localStorage.getItem(key);
	} catch {
		return null;
	}
}

function write(key: string, value: string): void {
	if (!browser) return;
	try {
		window.localStorage.setItem(key, value);
	} catch {
		// Site data is blocked. The scope applies for this page view and is not
		// remembered; nothing else about the screen changes.
	}
}

/** Parse a stored id list. Anything unparseable is dropped rather than
 * throwing: a corrupt preference must not be able to break the screen. */
export function parseIds(raw: string | null): number[] {
	if (!raw) return [];
	return raw
		.split(',')
		.map((part) => Number.parseInt(part, 10))
		.filter((n) => Number.isSafeInteger(n) && n >= 0);
}

function parseKnown(raw: string | null): KnownIndexer[] {
	if (!raw) return [];
	try {
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed.flatMap((entry) => {
			if (typeof entry !== 'object' || entry === null) return [];
			const { id, name } = entry as { id?: unknown; name?: unknown };
			if (typeof id !== 'number' || !Number.isSafeInteger(id)) return [];
			return [{ id, name: typeof name === 'string' && name ? name : `indexer ${id}` }];
		});
	} catch {
		return [];
	}
}

export function createIndexerScope() {
	let known = $state<KnownIndexer[]>(SELECTION_PERSISTS ? parseKnown(read(KNOWN_KEY)) : []);
	let selected = $state<number[]>(SELECTION_PERSISTS ? parseIds(read(SELECTION_KEY)) : []);

	/** An EMPTY selection means every indexer, which is the server's own
	 * default. It is not the same as a one-element list and the two must never
	 * be collapsed. */
	const all = $derived(selected.length === 0);

	function persistSelection(): void {
		if (!SELECTION_PERSISTS) return;
		write(SELECTION_KEY, selected.join(','));
	}

	return {
		get known(): KnownIndexer[] {
			// Sorted by name so the picker does not reshuffle as new indexers
			// report. Ids arrive in fan-out order, which is not an order a person
			// can hold in their head.
			return [...known].sort((a, b) => a.name.localeCompare(b.name));
		},
		get selected(): number[] {
			return selected;
		},
		get isAll(): boolean {
			return all;
		},
		isSelected(id: number): boolean {
			return selected.includes(id);
		},

		/**
		 * Learn an indexer from a search frame. The catalogue only ever grows —
		 * see the file header: a scoped search reports only the indexers it was
		 * scoped to, and narrowing on that would delete the rest of the list the
		 * moment someone used the filter.
		 */
		learn(id: number, name: string): void {
			if (!Number.isSafeInteger(id) || id < 0) return;
			const existing = known.find((k) => k.id === id);
			if (existing && existing.name === name) return;
			known = [...known.filter((k) => k.id !== id), { id, name: name || `indexer ${id}` }];
			if (SELECTION_PERSISTS) write(KNOWN_KEY, JSON.stringify(known));
		},

		toggle(id: number): void {
			selected = selected.includes(id) ? selected.filter((n) => n !== id) : [...selected, id];
			persistSelection();
		},

		/** Back to every indexer. The control that does this is what makes a
		 * sticky filter recoverable without opening the picker. */
		clear(): void {
			selected = [];
			persistSelection();
		},

		/** Adopt a selection from the URL, so a shared link opens scoped. */
		adopt(ids: number[]): void {
			selected = ids.filter((n) => Number.isSafeInteger(n) && n >= 0);
			persistSelection();
		},

		/**
		 * THE VISIBLE STATEMENT OF SCOPE. Empty string when the scope is the
		 * default, because a screen that says "all indexers" on every search has
		 * taught the reader to stop reading the line by the time it matters.
		 */
		get summary(): string {
			if (all) return '';
			const names = selected
				.map((id) => known.find((k) => k.id === id)?.name ?? `indexer ${id}`)
				.sort((a, b) => a.localeCompare(b));
			if (names.length === 1) return `Searching ${names[0]} only.`;
			if (names.length <= 3) return `Searching ${names.join(', ')} only.`;
			return `Searching ${names.length} of ${known.length} known indexers.`;
		}
	};
}

export type IndexerScope = ReturnType<typeof createIndexerScope>;
