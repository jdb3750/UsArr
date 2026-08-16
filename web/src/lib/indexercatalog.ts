/**
 * THE INDEXER CATALOGUE'S PURE HALF — the shaping the picker on Requests does
 * to `GET /api/v1/indexers` before it draws anything.
 *
 * ⚠️ WHAT THIS IS FOR, AND WHY IT IS A MODULE RATHER THAN TEMPLATE LOGIC.
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so anything
 * that lives in `+page.svelte` cannot be asserted at all — the limit
 * `requests.test.ts` and `list.test.ts` both record. The three things on this
 * path that can be wrong in a way a reader would never notice are all here:
 * which indexers are offered, how the raw Newznab tree is folded into something
 * a person can pick from, and which of the endpoint's four states offers a
 * control that actually leads somewhere.
 *
 * ⚠️ THE ENDPOINT IS A TIER 0 LOCAL READ AND MUST BE TREATED AS ONE.
 * `internal/httpapi/indexers.go` reads `indexer_catalog` — a replica the
 * background prober refreshes — and makes no upstream call, precisely so the
 * picker can paint before any search has run (ARCHITECTURE.md §8.5, §2.3 rule
 * 1). So nothing here has a loading shape, no caller may draw a skeleton over
 * it, and the render must never wait on it.
 *
 * ⚠️ ALL FOUR STATES ARRIVE ON A 200, AND THE BRANCH IS ON `status`, NEVER ON
 * THE HTTP CODE. "No indexer service configured" and "configured but never
 * successfully read" are states of an install rather than failures; a picker
 * that treated them as errors would render an error box on a screen where
 * everything else works. `catalogGuidance` below is the whole of that branch.
 */

/**
 * The catalogue statuses, verbatim from `internal/httpapi/indexers.go`. They are
 * a wire contract like an error code, and they travel on a 200.
 */
export const CATALOG_OK = 'ok';
export const CATALOG_NOT_CONFIGURED = 'not_configured';
export const CATALOG_NEVER_FETCHED = 'never_fetched';
export const CATALOG_EMPTY = 'empty';
export const CATALOG_SERVICE_DISABLED = 'service_disabled';

/** One Newznab/Torznab category, as the tree's own shape. NEVER collapsed into
 * invented buckets: 3030 is the only reliable machine signal separating an
 * audiobook from music and 7030 likewise for comics, so a bucket that merged
 * them would destroy the one thing the ids are good for (§8.5). */
export interface CatalogCategory {
	id: number;
	name: string;
	children: CatalogCategory[];
}

/** One indexer, as the catalogue endpoint describes it. */
export interface CatalogIndexer {
	instanceId: number;
	instanceName: string;
	indexerId: number;
	name: string;
	protocol: string;
	privacy: string;
	/** Routinely false. The upstream endpoint has no filter parameter, so a
	 * disabled indexer is LISTED and marked rather than hidden. */
	enabled: boolean;
	/** 1-50, and LOWER WINS. See `PRIORITY_NOTE`. */
	priority: number;
	supportsSearch: boolean;
	searchTypes: string[];
	categories: CatalogCategory[];
	fetchedAt: string;
}

/** One indexer service's contribution, without its indexers. */
export interface CatalogInstance {
	instanceId: number;
	name: string;
	kind: string;
	enabled: boolean;
	status: string;
	message: string;
	action: string;
	/** Absent when UsArr has never successfully read this instance's indexer
	 * list — which is exactly what separates `never_fetched` from `empty`. */
	fetchedAt?: string;
	indexerCount: number;
}

export interface IndexerCatalog {
	status: string;
	message: string;
	action: string;
	instances: CatalogInstance[];
	indexers: CatalogIndexer[];
}

/* ── 1. Which indexers are offered, and in what order ──────────────────────── */

/**
 * The picker's order: searchable first, then by name.
 *
 * ⚠️ PRIORITY IS DELIBERATELY NOT THE SORT KEY, and the reason is worth stating
 * because the field invites it. A picker is a LOOKUP surface — the user arrives
 * knowing which tracker they want and needs to find its name — and name order is
 * the only order a person can predict. Sorting by priority would also be the one
 * numeric sort on this screen that runs the other way (see `PRIORITY_NOTE`), so
 * the column that looked most sortable would be the one whose direction nobody
 * could guess.
 *
 * An indexer that cannot be searched sinks rather than disappearing: a disabled
 * indexer, and one that is RSS-only, are both still listed — see
 * `unavailableReason`. Hiding them makes "why is my indexer missing?"
 * unanswerable from the one screen that should answer it.
 */
export function sortIndexers(indexers: readonly CatalogIndexer[]): CatalogIndexer[] {
	return [...indexers].sort((a, b) => {
		const aOff = isSearchable(a) ? 0 : 1;
		const bOff = isSearchable(b) ? 0 : 1;
		if (aOff !== bOff) return aOff - bOff;
		return a.name.localeCompare(b.name) || a.indexerId - b.indexerId;
	});
}

/** Whether a search would actually ask this indexer. Both halves are the
 * planner's own rules (`internal/releases/search.go`): a disabled indexer is
 * never in the fan-out, and an RSS-only one is skipped as unsupported. */
export function isSearchable(indexer: CatalogIndexer): boolean {
	return indexer.enabled && indexer.supportsSearch;
}

/**
 * Why an indexer is listed but cannot be searched, or an empty string when it
 * can be.
 *
 * It is a fact rather than a scolding, and it never guesses: these are the only
 * two conditions the planner skips on before it has asked anything.
 */
export function unavailableReason(indexer: CatalogIndexer): string {
	if (!indexer.enabled) return 'Turned off in this indexer service, so a search never asks it.';
	if (!indexer.supportsSearch) return 'RSS only — it advertises no search, so a search skips it.';
	return '';
}

/**
 * ⚠️ LOWER WINS, AND THE PANEL SAYS SO EXACTLY ONCE.
 *
 * Prowlarr's priority is 1-50 with 25 as the default, and the SMALLER number is
 * the preferred one — `servarr.DedupeReleases` keeps the copy from the indexer
 * with the lowest value, and UsArr matches Prowlarr's own rule so the two UIs
 * agree about the same query. A bare "Priority 5" beside a "Priority 40"
 * therefore reads backwards to anyone who has met a priority field anywhere
 * else, so the number may never appear without the rule that orders it.
 *
 * It appears ONCE, in the panel, rather than on every row: the clause is
 * identical for every indexer, and DESIGN-DIRECTION §9.1 is explicit that a
 * value identical across every row of a group is not data — state it once and
 * drop it from the rows. Measured, it was three wrapped lines under every name
 * in a four-column grid, which is more ink than the names themselves.
 *
 * And it is a DE-DUPLICATION tiebreak, not a fan-out order: UsArr asks every
 * selected indexer in parallel, so nothing here is "asked first".
 */
export const PRIORITY_NOTE =
	'Priority is the tiebreak when two indexers answer with the same release, and the lower number wins.';

/**
 * One indexer's facts, in its own vocabulary: protocol, privacy and priority,
 * each present only when the catalogue has it. Upstream words are rendered
 * verbatim and never re-spelled.
 */
export function indexerFacts(indexer: CatalogIndexer): string {
	const priority =
		Number.isFinite(indexer.priority) && indexer.priority > 0 ? `priority ${indexer.priority}` : '';
	return [indexer.protocol, indexer.privacy, priority].filter((s) => s !== '').join(' · ');
}

/* ── 2. The category tree ─────────────────────────────────────────────────── */

/**
 * The categories a search of THESE indexers could be narrowed to.
 *
 * ⚠️ THE TREE'S OWN STRUCTURE, AND NEITHER OF THE TWO WAYS TO GET IT WRONG.
 * Flattening it hands the user a couple of hundred raw Newznab ids, which is not
 * a control. Collapsing it into invented buckets — "audio", "books" — throws
 * away the leaf ids that are the only reliable signal there is: 3030 sits under
 * Audio (3000) and is the sole machine-readable mark of an audiobook, and 7030
 * likewise for comics (§8.5). So the two levels the tree already has are the two
 * levels the picker draws: the parents are the short path, and each one opens on
 * its own children.
 *
 * SCOPED TO THE SELECTED INDEXERS, which is what makes "one tracker, one
 * category" two decisions instead of a search through everything every indexer
 * on the install has ever advertised. With nothing selected it is the union over
 * every searchable indexer, because that is what an unscoped search would ask.
 *
 * A parent id is worth offering on its own: the server matches a requested
 * category against an indexer's advertised tree in BOTH directions —
 * `supportsAnyCategory` walks children and consults `mapping.ParentCategory` —
 * so selecting `2000` reaches an indexer that only advertises `2045`.
 *
 * Sorted by id, which is the standard's own order and therefore stable: the
 * picker must not reshuffle when a second indexer's categories merge in.
 */
export function categoryTree(
	indexers: readonly CatalogIndexer[],
	selectedIndexerIds: readonly number[] = []
): CatalogCategory[] {
	const wanted = new Set(selectedIndexerIds);
	const roots = new Map<number, { name: string; children: Map<number, string> }>();

	for (const indexer of indexers) {
		if (!isSearchable(indexer)) continue;
		if (wanted.size > 0 && !wanted.has(indexer.indexerId)) continue;
		for (const parent of indexer.categories) {
			const root = roots.get(parent.id) ?? { name: parent.name, children: new Map() };
			// First non-empty name wins. Two indexers naming 2000 differently is
			// ordinary; picking the later one would make the list flicker between
			// installs for no gain.
			if (root.name === '' && parent.name !== '') root.name = parent.name;
			for (const child of parent.children) {
				if (!root.children.has(child.id) || root.children.get(child.id) === '') {
					root.children.set(child.id, child.name);
				}
			}
			roots.set(parent.id, root);
		}
	}

	return [...roots.entries()]
		.sort((a, b) => a[0] - b[0])
		.map(([id, root]) => ({
			id,
			name: root.name,
			children: [...root.children.entries()]
				.sort((a, b) => a[0] - b[0])
				.map(([childId, name]) => ({ id: childId, name, children: [] }))
		}));
}

/**
 * A category's label. The upstream name verbatim where there is one — Prowlarr's
 * own `Movies/HD` rather than a prettier re-spelling UsArr invented — and the
 * bare id where there is not, because a category with no name is still a real
 * filter and dropping it would silently narrow what can be picked.
 */
export function categoryLabelFor(category: { id: number; name: string }): string {
	return category.name !== '' ? category.name : `Category ${category.id}`;
}

/**
 * Names for the scope sentence, catalogue first and the SSE-learned union
 * behind it.
 *
 * The order is the whole of it: the catalogue is authoritative, and a learned
 * entry is a leftover that may name an indexer which no longer exists. A
 * selection neither source knows still has to be describable, so the caller
 * falls back to the bare id rather than this dropping it.
 */
export function indexerNames(
	catalogue: readonly CatalogIndexer[],
	learned: readonly { id: number; name: string }[]
): Map<number, string> {
	const out = new Map<number, string>();
	for (const known of learned) out.set(known.id, known.name);
	for (const ix of catalogue) out.set(ix.indexerId, ix.name);
	return out;
}

/** Every id in the tree, parents and children alike, for resolving a selection
 * that came out of the URL or out of storage. */
export function categoryNames(tree: readonly CatalogCategory[]): Map<number, string> {
	const out = new Map<number, string>();
	for (const parent of tree) {
		out.set(parent.id, categoryLabelFor(parent));
		for (const child of parent.children) out.set(child.id, categoryLabelFor(child));
	}
	return out;
}

/* ── 3. The four states, and which of them has a control that works ────────── */

/** What a non-`ok` catalogue offers the user. */
export interface CatalogGuidance {
	/** Whether the fix is on UsArr's own Services screen. Where it is not, the
	 * sentence is rendered with NO button: §17.5's "naming the non-action beats
	 * offering a fake one", and a control that opens a screen which correctly
	 * shows everything green is exactly the fake one it means. */
	fixInServices: boolean;
}

/**
 * ⚠️ `empty` IS THE ONE STATE WHOSE FIX IS NOT IN UsArr. The service answered
 * and has no indexers configured, so the thing to do is add one IN PROWLARR —
 * UsArr has no surface for that, will not grow one, and Services would show the
 * connection as healthy because it is. The other three are all UsArr's own
 * configuration: nothing added, something switched off, or a connection that has
 * not yet produced a read.
 */
export function catalogGuidance(status: string): CatalogGuidance {
	return { fixInServices: status !== CATALOG_OK && status !== CATALOG_EMPTY };
}

/* ── 4. The visible statement of scope ────────────────────────────────────── */

export interface ScopeNames {
	/** The selected indexers, already resolved to names. */
	indexers: readonly { id: number; name: string }[];
	/** The selected categories, already resolved to names. */
	categories: readonly { id: number; name: string }[];
	/** How many indexers the picker is offering, for the "n of m" form. */
	knownIndexers: number;
}

/**
 * THE SENTENCE THAT MAKES A STICKY FILTER RECOVERABLE.
 *
 * Empty string when the scope is the default, because a line that says "all
 * indexers" on every search has taught the reader to skip it by the time it
 * matters. Whenever it is anything else, the scope is stated in words next to
 * the search box rather than only inside a picker somebody has to open — a
 * filter that silently persists across sessions is how a user concludes an
 * indexer is broken when they simply left it deselected weeks ago.
 *
 * It lives here rather than in `indexerscope.svelte` for one reason: a `.svelte`
 * module cannot be imported by a `environment: 'node'` vitest run, and this is
 * the copy most worth pinning.
 */
export function scopeSummary(names: ScopeNames): string {
	const parts: string[] = [];
	if (names.indexers.length > 0) parts.push(namedScope(names.indexers, names.knownIndexers));
	if (names.categories.length > 0) parts.push(`in ${namedCategories(names.categories)}`);
	if (parts.length === 0) return '';
	return `Searching ${parts.join(', ')}.`;
}

function namedScope(indexers: readonly { id: number; name: string }[], known: number): string {
	const list = [...indexers].map((i) => i.name).sort((a, b) => a.localeCompare(b));
	if (list.length <= 3) return `${list.join(', ')} only`;
	// Past three names the list stops being readable at a glance and the useful
	// fact is the proportion. `known` can be short of the selection when a stored
	// id is no longer in the catalogue, so it is floored rather than trusted.
	return `${list.length} of ${Math.max(known, list.length)} indexers`;
}

/**
 * The category half always carries the noun, and that is not a style choice:
 * "Searching Movies." is indistinguishable from a report of the query someone
 * typed, and this line's whole job is to be readable as a statement about the
 * FILTER by somebody who has forgotten they set one.
 */
function namedCategories(categories: readonly { id: number; name: string }[]): string {
	const list = [...categories].map((c) => c.name).sort((a, b) => a.localeCompare(b));
	if (list.length === 1) return `the ${list[0]} category`;
	if (list.length <= 3) return `the ${list.slice(0, -1).join(', ')} and ${list.at(-1)} categories`;
	return `${list.length} categories`;
}
