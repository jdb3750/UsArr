/**
 * WHAT A SEARCH IS ALLOWED TO ASK — which indexers, and which categories.
 *
 * The server already accepts `?indexer=` and `?category=` repeated and narrows
 * the fan-out BEFORE the legs are planned (internal/httpapi/search.go →
 * releases.Query), so an unselected indexer is never asked and spends none of
 * its rate limit, and an indexer carrying none of the requested categories is
 * skipped as unsupported rather than queried and discarded. The client was
 * sending only `?query=`, which is why a Prowlarr carrying a dozen indexers
 * answered every search with everything.
 *
 * ⚠️ WHERE THE LIST OF INDEXERS COMES FROM. THIS COMMENT USED TO SAY THERE WAS
 * NO ENDPOINT FOR IT. That was true when it was written and is now false, and it
 * was the actively dangerous kind of false: it told whoever came to wire the
 * picker that the thing they needed did not exist.
 *
 * `GET /api/v1/indexers` (`internal/httpapi/indexers.go`, registered in
 * `server.go`) is the catalogue, and `$lib/api.fetchIndexerCatalog` reads it. It
 * is a TIER 0 LOCAL REPLICA read over `indexer_catalog` — the background prober
 * refreshes it, the handler makes no upstream call — so the picker is populated
 * on FIRST RENDER, before any search has run, which is the whole point of it
 * being a replica (ARCHITECTURE.md §8.5, §2.3 rule 1). Trustworthy from
 * `70a61c9` onward, which is the two-snapshot fix.
 *
 * SO WHAT IS THE `learn` PATH BELOW STILL FOR? Exactly one state, and it is a
 * real one: `never_fetched`. UsArr can search a Prowlarr it has not yet probed
 * — the search path asks upstream, the catalogue path reads a replica that may
 * still be empty — and in that window the SSE frames are the ONLY names there
 * are. Every `search.indexer` frame names an indexer as the fan-out starts it,
 * and the closing `search.done` report names every indexer including those
 * skipped or blocked, so one unscoped search still yields the full set. It is a
 * FALLBACK now rather than the source: the catalogue wins outright whenever it
 * has rows, so an indexer deleted upstream stops being offered instead of
 * lingering in localStorage for ever.
 *
 * STORAGE KEYS ARE PART OF THE CONTRACT, exactly as in `$lib/prefs.svelte`:
 * `usarr.search.indexers` and `usarr.search.known-indexers` are what a browser
 * that has already been used will have written. Renaming one silently resets
 * every existing install, so they are frozen. `usarr.search.categories` is new
 * and therefore resets nothing; it is frozen from here on for the same reason.
 * Reads and writes both go through try/catch — localStorage throws rather than
 * returning null in a browser with site data blocked, and a filter is never
 * worth taking the screen down for.
 */
import { browser } from '$app/environment';

export const SELECTION_KEY = 'usarr.search.indexers';
export const KNOWN_KEY = 'usarr.search.known-indexers';
export const CATEGORY_KEY = 'usarr.search.categories';

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
 * other than "all indexers, all categories", the screen states it in words next
 * to the search box — `$lib/indexercatalog.scopeSummary` writes that sentence,
 * where a `environment: 'node'` test can read it.
 *
 * ⚠️ IT GOVERNS THE CATEGORY SCOPE TOO, and deliberately so. Two filters on one
 * control with two different memories is a screen nobody can predict, and the
 * category filter is the one MORE likely to be forgotten — an indexer name is
 * recognisable, "the Movies category" reads like something the search did rather
 * than something the user set. Same switch, same sentence, same clear control.
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
	let learned = $state<KnownIndexer[]>(SELECTION_PERSISTS ? parseKnown(read(KNOWN_KEY)) : []);
	let selected = $state<number[]>(SELECTION_PERSISTS ? parseIds(read(SELECTION_KEY)) : []);
	let categories = $state<number[]>(SELECTION_PERSISTS ? parseIds(read(CATEGORY_KEY)) : []);

	/** An EMPTY selection means every indexer, which is the server's own
	 * default. It is not the same as a one-element list and the two must never
	 * be collapsed. The same holds for categories. */
	const all = $derived(selected.length === 0);
	const allCategories = $derived(categories.length === 0);

	function persistSelection(): void {
		if (!SELECTION_PERSISTS) return;
		write(SELECTION_KEY, selected.join(','));
	}

	function persistCategories(): void {
		if (!SELECTION_PERSISTS) return;
		write(CATEGORY_KEY, categories.join(','));
	}

	return {
		/**
		 * THE FALLBACK CATALOGUE ONLY — the indexers SSE frames have named.
		 *
		 * The picker draws `GET /api/v1/indexers` when that has rows; this covers
		 * the `never_fetched` window, where a search works and the replica is
		 * still empty. Sorted by name so it does not reshuffle as frames land:
		 * ids arrive in fan-out order, which is not an order a person can hold in
		 * their head.
		 */
		get known(): KnownIndexer[] {
			return [...learned].sort((a, b) => a.name.localeCompare(b.name));
		},
		get selected(): number[] {
			return selected;
		},
		get categories(): number[] {
			return categories;
		},
		get isAll(): boolean {
			return all;
		},
		get isAllCategories(): boolean {
			return allCategories;
		},
		isSelected(id: number): boolean {
			return selected.includes(id);
		},
		isCategorySelected(id: number): boolean {
			return categories.includes(id);
		},

		/**
		 * Learn an indexer from a search frame. This union only ever grows — a
		 * scoped search reports only the indexers it was scoped to, and narrowing
		 * on that would delete the rest of the list the moment someone used the
		 * filter. It is bounded by the authoritative catalogue taking precedence
		 * over it the instant the replica has anything in it; see the header.
		 */
		learn(id: number, name: string): void {
			if (!Number.isSafeInteger(id) || id < 0) return;
			const existing = learned.find((k) => k.id === id);
			if (existing && existing.name === name) return;
			learned = [...learned.filter((k) => k.id !== id), { id, name: name || `indexer ${id}` }];
			if (SELECTION_PERSISTS) write(KNOWN_KEY, JSON.stringify(learned));
		},

		toggle(id: number): void {
			selected = selected.includes(id) ? selected.filter((n) => n !== id) : [...selected, id];
			persistSelection();
		},

		toggleCategory(id: number): void {
			categories = categories.includes(id)
				? categories.filter((n) => n !== id)
				: [...categories, id];
			persistCategories();
		},

		/** Back to every indexer. The control that does this is what makes a
		 * sticky filter recoverable without opening the picker. */
		clear(): void {
			selected = [];
			persistSelection();
		},

		clearCategories(): void {
			categories = [];
			persistCategories();
		},

		/** Both filters at once, for the one control beside the scope sentence.
		 * A user who has forgotten they set a scope should not have to discover
		 * that it has two halves before they can get back to a plain search. */
		clearAll(): void {
			selected = [];
			categories = [];
			persistSelection();
			persistCategories();
		},

		/** Adopt a selection from the URL, so a shared link opens scoped. */
		adopt(ids: number[]): void {
			selected = ids.filter((n) => Number.isSafeInteger(n) && n >= 0);
			persistSelection();
		},

		adoptCategories(ids: number[]): void {
			categories = ids.filter((n) => Number.isSafeInteger(n) && n >= 0);
			persistCategories();
		}
	};
}

export type IndexerScope = ReturnType<typeof createIndexerScope>;
