/**
 * WHAT A SEARCH IS ALLOWED TO ASK — which indexer service, which indexers
 * inside it, and which categories.
 *
 * The server already accepts `?indexer=` and `?category=` repeated and narrows
 * the fan-out BEFORE the legs are planned (internal/httpapi/search.go →
 * releases.Query), so an unselected indexer is never asked and spends none of
 * its rate limit, and an indexer carrying none of the requested categories is
 * skipped as unsupported rather than queried and discarded. The client was
 * sending only `?query=`, which is why a Prowlarr carrying a dozen indexers
 * answered every search with everything.
 *
 * ⚠️ AND IT ACCEPTS `?instance=`, WHICH THE CLIENT ALSO WAS NOT SENDING. That
 * omission is the more serious of the two, because it was silent:
 * `resolveIndexerInstance` falls back to `candidates[0]` by priority-then-name,
 * so an install with two indexer services had every search answered by one of
 * them and the other never contacted at all — under a picker that listed both
 * services' indexers together as though a search covered them. The scope below
 * therefore carries an ACTIVE INSTANCE, everything else is relative to it, and
 * `$lib/indexercatalog`'s header owns the argument for why one service per
 * search is the honest shape rather than a limitation worked around.
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
 * A learned entry is filed under the instance the search actually ran against —
 * `GET /api/v1/releases/search` returns `instance_id` in its accepted body, so
 * the client is told rather than guessing — for the same reason the selection
 * is: an indexer id from one Prowlarr means nothing in another.
 *
 * ⚠️ THAT LINE USED TO READ `POST /api/v1/search`, AND BOTH HALVES WERE WRONG.
 * The verb was never right — `internal/httpapi/server.go` has only ever routed
 * the fan-out on GET — and the PATH stopped being right at `4a51bd4`, which
 * moved the release fan-out to `/api/v1/releases/search` so that `04a28a4`
 * could give `/api/v1/search` to the library search (`docs/reference/
 * http-api.md` §6). The two are different questions over different corpora and
 * nothing in this module speaks to the library one.
 *
 * ⚠️ STORAGE KEYS ARE PART OF THE CONTRACT, exactly as in `$lib/prefs.svelte`.
 * `usarr.search.categories` is unchanged and stays frozen: a Newznab category
 * id is a standard's id, identical in every service, so nothing about it was
 * ambiguous. The two indexer keys ARE new, and `RETIRED_KEYS` records the
 * deliberate reset that came with them. Reads and writes both go through
 * try/catch — localStorage throws rather than returning null in a browser with
 * site data blocked, and a filter is never worth taking the screen down for.
 */
import { browser } from '$app/environment';
import { formatPairs, pairsForInstance, parsePairs, type IndexerPair } from '$lib/indexercatalog';

export const SELECTION_KEY = 'usarr.search.indexer-pairs';
export const KNOWN_KEY = 'usarr.search.known-indexer-pairs';
export const INSTANCE_KEY = 'usarr.search.indexer-service';
export const CATEGORY_KEY = 'usarr.search.categories';

/**
 * ⚠️ THE OLD KEYS, AND WHY THIS IS A RESET RATHER THAN A MIGRATION.
 *
 * `usarr.search.indexers` held `3,5` and `usarr.search.known-indexers` held
 * `[{id,name}]`. Neither carries a service, and an indexer id without one is
 * ambiguous the moment a second service exists — which is the entire bug this
 * shape fixes.
 *
 * A migration was available and was REFUSED. The tempting one is "the
 * catalogue has exactly one service, so the stored ids can only have meant
 * that one, adopt them under it". It is even true for most installs. But the
 * install it is NOT true for is the two-service install, which is precisely the
 * one whose selection was already meaningless, and the failure mode there is a
 * search silently narrowed to a set of indexers the user never picked. Reading
 * a stored value under a meaning it did not have is the class of bug being
 * fixed here, so doing it once more on the way out would be the wrong trade for
 * saving one re-tick.
 *
 * The reset lands on the empty selection, which is "every indexer in the active
 * service" — never narrower than the user expects, and stated in words by
 * `scopeSummary` the moment it is not the whole picture.
 *
 * They are actively DELETED rather than left to rot, so that no later revert or
 * half-revert can find them and read them under the old meaning again. Deleting
 * a key nobody reads is cheap; a stale one that comes back to life is not.
 */
export const RETIRED_KEYS = ['usarr.search.indexers', 'usarr.search.known-indexers'] as const;

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
 * where a `environment: 'node'` test can read it. With more than one indexer
 * service configured that sentence is ALWAYS rendered, because "the default"
 * then means "one of your two services" and nobody would guess which.
 *
 * ⚠️ IT GOVERNS THE CATEGORY SCOPE AND THE CHOSEN SERVICE TOO, and deliberately
 * so. Several filters on one control with several different memories is a
 * screen nobody can predict, and the category filter is the one MORE likely to
 * be forgotten — an indexer name is recognisable, "the Movies category" reads
 * like something the search did rather than something the user set. Same
 * switch, same sentence, same clear control.
 */
export const SELECTION_PERSISTS = true;

/** One indexer, as a search has reported it, filed under the service that
 * reported it. */
export interface KnownIndexer {
	instanceId: number;
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

function drop(key: string): void {
	if (!browser) return;
	try {
		window.localStorage.removeItem(key);
	} catch {
		// Same as `write`: nothing here is worth taking the screen down for.
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

function parseInstance(raw: string | null): number {
	const id = Number.parseInt(raw ?? '', 10);
	return Number.isSafeInteger(id) && id > 0 ? id : 0;
}

function parseKnown(raw: string | null): KnownIndexer[] {
	if (!raw) return [];
	try {
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed.flatMap((entry) => {
			if (typeof entry !== 'object' || entry === null) return [];
			const { instanceId, id, name } = entry as {
				instanceId?: unknown;
				id?: unknown;
				name?: unknown;
			};
			if (typeof id !== 'number' || !Number.isSafeInteger(id)) return [];
			// No instance, no entry. The learned union is a fallback catalogue and
			// an unattributed name in it would offer the same false choice the
			// retired selection key did.
			if (typeof instanceId !== 'number' || !Number.isSafeInteger(instanceId)) return [];
			if (instanceId <= 0) return [];
			return [{ instanceId, id, name: typeof name === 'string' && name ? name : `indexer ${id}` }];
		});
	} catch {
		return [];
	}
}

export function createIndexerScope() {
	// The one-time cleanup of the ambiguous shape. See `RETIRED_KEYS`.
	for (const key of RETIRED_KEYS) drop(key);

	let learned = $state<KnownIndexer[]>(SELECTION_PERSISTS ? parseKnown(read(KNOWN_KEY)) : []);
	let pairs = $state<IndexerPair[]>(SELECTION_PERSISTS ? parsePairs(read(SELECTION_KEY)) : []);
	let categories = $state<number[]>(SELECTION_PERSISTS ? parseIds(read(CATEGORY_KEY)) : []);
	// 0 until the catalogue (or a link, or a search's own answer) names one.
	let instanceId = $state<number>(SELECTION_PERSISTS ? parseInstance(read(INSTANCE_KEY)) : 0);

	/** The active service's selection — which is what a search sends, because a
	 * search asks one service. */
	const selected = $derived(pairsForInstance(pairs, instanceId));

	/** An EMPTY selection means every indexer IN THE ACTIVE SERVICE, which is
	 * the server's own default once `?instance=` is set. It is not the same as a
	 * one-element list and the two must never be collapsed. The same holds for
	 * categories. */
	const all = $derived(selected.length === 0);
	const allCategories = $derived(categories.length === 0);

	function persistSelection(): void {
		if (!SELECTION_PERSISTS) return;
		write(SELECTION_KEY, formatPairs(pairs));
	}

	function persistCategories(): void {
		if (!SELECTION_PERSISTS) return;
		write(CATEGORY_KEY, categories.join(','));
	}

	return {
		/**
		 * THE FALLBACK CATALOGUE ONLY — the indexers SSE frames have named, for
		 * the ACTIVE service.
		 *
		 * The picker draws `GET /api/v1/indexers` when that has rows; this covers
		 * the `never_fetched` window, where a search works and the replica is
		 * still empty. Sorted by name so it does not reshuffle as frames land:
		 * ids arrive in fan-out order, which is not an order a person can hold in
		 * their head.
		 */
		get known(): KnownIndexer[] {
			return learned
				.filter((k) => k.instanceId === instanceId)
				.sort((a, b) => a.name.localeCompare(b.name));
		},
		get selected(): number[] {
			return selected;
		},
		get categories(): number[] {
			return categories;
		},
		get instanceId(): number {
			return instanceId;
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
		 * Point the whole scope at one indexer service.
		 *
		 * The other services' ticks are KEPT rather than cleared, and that is the
		 * point of storing pairs: switching back restores what was picked there,
		 * and nothing the user set is thrown away by a control that only said
		 * "show me this one". Only the active service's pairs ever reach a
		 * request, so the retained ones cannot widen or narrow a search behind
		 * anyone's back.
		 */
		setInstance(id: number): void {
			const next = Number.isSafeInteger(id) && id > 0 ? id : 0;
			if (next === instanceId) return;
			instanceId = next;
			if (SELECTION_PERSISTS) write(INSTANCE_KEY, String(next));
		},

		/**
		 * Learn an indexer from a search frame, under the service that ran the
		 * search. This union only ever grows — a scoped search reports only the
		 * indexers it was scoped to, and narrowing on that would delete the rest
		 * of the list the moment someone used the filter. It is bounded by the
		 * authoritative catalogue taking precedence over it the instant the
		 * replica has anything in it; see the header.
		 */
		learn(instance: number, id: number, name: string): void {
			if (!Number.isSafeInteger(id) || id < 0) return;
			if (!Number.isSafeInteger(instance) || instance <= 0) return;
			const existing = learned.find((k) => k.instanceId === instance && k.id === id);
			if (existing && existing.name === name) return;
			learned = [
				...learned.filter((k) => !(k.instanceId === instance && k.id === id)),
				{ instanceId: instance, id, name: name || `indexer ${id}` }
			];
			if (SELECTION_PERSISTS) write(KNOWN_KEY, JSON.stringify(learned));
		},

		/** Tick one indexer OF THE ACTIVE SERVICE. Without an active service
		 * there is no pair to store, and a tick that stored `0:3` would be the
		 * ambiguous shape again under a new name. */
		toggle(id: number): void {
			if (instanceId <= 0) return;
			pairs = selected.includes(id)
				? pairs.filter((p) => !(p.instanceId === instanceId && p.indexerId === id))
				: [...pairs, { instanceId, indexerId: id }];
			persistSelection();
		},

		toggleCategory(id: number): void {
			categories = categories.includes(id)
				? categories.filter((n) => n !== id)
				: [...categories, id];
			persistCategories();
		},

		/** Back to every indexer in the active service. The control that does
		 * this is what makes a sticky filter recoverable without opening the
		 * picker. */
		clear(): void {
			pairs = pairs.filter((p) => p.instanceId !== instanceId);
			persistSelection();
		},

		clearCategories(): void {
			categories = [];
			persistCategories();
		},

		/** Both filters at once, for the one control beside the scope sentence.
		 * A user who has forgotten they set a scope should not have to discover
		 * that it has two halves before they can get back to a plain search.
		 *
		 * It clears the ACTIVE service's ticks only, because that is the whole of
		 * what the sentence beside it describes — a button that also wiped a
		 * selection on a service the sentence never mentioned would be doing
		 * something the user could not see. */
		clearAll(): void {
			pairs = pairs.filter((p) => p.instanceId !== instanceId);
			categories = [];
			persistSelection();
			persistCategories();
		},

		/** Adopt a selection from the URL, so a shared link opens scoped. The ids
		 * are attributed to the service the same link named. */
		adopt(instance: number, ids: number[]): void {
			if (!Number.isSafeInteger(instance) || instance <= 0) return;
			const wanted = ids.filter((n) => Number.isSafeInteger(n) && n >= 0);
			pairs = [
				...pairs.filter((p) => p.instanceId !== instance),
				...wanted.map((indexerId) => ({ instanceId: instance, indexerId }))
			];
			persistSelection();
		},

		adoptCategories(ids: number[]): void {
			categories = ids.filter((n) => Number.isSafeInteger(n) && n >= 0);
			persistCategories();
		}
	};
}

export type IndexerScope = ReturnType<typeof createIndexerScope>;
