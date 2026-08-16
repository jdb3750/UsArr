/**
 * The indexer catalogue's pure half.
 *
 * Everything asserted here is something a reader would not notice being wrong:
 * a picker that silently drops a disabled indexer looks like a picker that is
 * simply short, a priority sorted the intuitive way looks like a priority sorted
 * correctly, and a category tree folded into buckets looks tidier than the tree
 * it destroyed. The template that composes these cannot be tested at all —
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin — so this is
 * where the rules live.
 */
import { describe, expect, it } from 'vitest';
import {
	CATALOG_EMPTY,
	CATALOG_NEVER_FETCHED,
	CATALOG_NOT_CONFIGURED,
	CATALOG_OK,
	CATALOG_SERVICE_DISABLED,
	catalogGuidance,
	categoryLabelFor,
	categoryNames,
	categoryTree,
	indexerFacts,
	indexerNames,
	isSearchable,
	PRIORITY_NOTE,
	scopeSummary,
	sortIndexers,
	unavailableReason,
	type CatalogIndexer
} from './indexercatalog';

function indexer(over: Partial<CatalogIndexer> = {}): CatalogIndexer {
	return {
		instanceId: 1,
		instanceName: 'Prowlarr',
		indexerId: 1,
		name: 'Indexer',
		protocol: 'usenet',
		privacy: 'public',
		enabled: true,
		priority: 25,
		supportsSearch: true,
		searchTypes: ['search'],
		categories: [],
		fetchedAt: '2026-08-16T12:00:00Z',
		...over
	};
}

describe('which indexers a search can ask', () => {
	it('treats a disabled indexer as unsearchable without dropping it', () => {
		// The upstream endpoint returns enabled AND disabled indexers with no
		// filter parameter, so this is an ordinary value rather than an edge case.
		const off = indexer({ enabled: false });
		expect(isSearchable(off)).toBe(false);
		expect(sortIndexers([off])).toHaveLength(1);
	});

	it('treats an RSS-only indexer as unsearchable, because the planner does', () => {
		// internal/releases/search.go skips it as OutcomeUnsupported before the leg
		// is planned. A picker that offered it would offer a tick that does nothing.
		expect(isSearchable(indexer({ supportsSearch: false }))).toBe(false);
	});

	it('names the reason, and names nothing when there is none', () => {
		expect(unavailableReason(indexer({ enabled: false }))).toContain('Turned off');
		expect(unavailableReason(indexer({ supportsSearch: false }))).toContain('RSS only');
		expect(unavailableReason(indexer())).toBe('');
	});

	it('sorts by name, and sinks what cannot be searched', () => {
		const rows = sortIndexers([
			indexer({ indexerId: 1, name: 'Zeta' }),
			indexer({ indexerId: 2, name: 'Alpha', enabled: false }),
			indexer({ indexerId: 3, name: 'Mid' })
		]);
		expect(rows.map((r) => r.name)).toEqual(['Mid', 'Zeta', 'Alpha']);
	});

	it('does not sort by priority, which would run the other way', () => {
		// 🔍 The trap this pins. Priority is 1-50 and LOWER WINS
		// (internal/servarr/resources.go, and DedupeReleases keeps the lowest), so
		// a priority sort would be the one numeric order on this screen that runs
		// ascending-is-best. A picker is a lookup surface and name order is the
		// only order a person can predict.
		const rows = sortIndexers([
			indexer({ indexerId: 1, name: 'Zeta', priority: 1 }),
			indexer({ indexerId: 2, name: 'Alpha', priority: 50 })
		]);
		expect(rows.map((r) => r.name)).toEqual(['Alpha', 'Zeta']);
	});

	it('never states a priority number without the rule that orders it', () => {
		// 🔍 A bare "priority 5" beside "priority 40" reads backwards to anyone who
		// has met a priority field anywhere else, so the number on the row is only
		// legible because the panel carries this sentence beside it.
		expect(indexerFacts(indexer({ priority: 5 }))).toContain('priority 5');
		expect(PRIORITY_NOTE).toContain('lower number wins');
		// And it is a de-duplication tiebreak, not a fan-out order: UsArr asks
		// every selected indexer in parallel.
		expect(PRIORITY_NOTE.toLowerCase()).not.toContain('first');
	});

	it('omits the number where the catalogue has none, rather than printing a zero', () => {
		expect(indexerFacts(indexer({ priority: 0 }))).toBe('usenet · public');
		expect(indexerFacts(indexer({ priority: Number.NaN }))).toBe('usenet · public');
	});

	it('joins the upstream words and neither invents nor pads them', () => {
		expect(indexerFacts(indexer())).toBe('usenet · public · priority 25');
		expect(indexerFacts(indexer({ privacy: '', priority: 0 }))).toBe('usenet');
		expect(indexerFacts(indexer({ protocol: '', privacy: '', priority: 0 }))).toBe('');
	});
});

describe('the category tree', () => {
	const movies = {
		id: 2000,
		name: 'Movies',
		children: [
			{ id: 2040, name: 'Movies/HD', children: [] },
			{ id: 2030, name: 'Movies/SD', children: [] }
		]
	};
	const audio = {
		id: 3000,
		name: 'Audio',
		children: [{ id: 3030, name: 'Audio/Audiobook', children: [] }]
	};

	it('keeps the tree’s own two levels rather than flattening or bucketing it', () => {
		const tree = categoryTree([indexer({ categories: [movies, audio] })]);
		expect(tree.map((c) => c.id)).toEqual([2000, 3000]);
		// 🔍 3030 survives as a leaf, which is the whole point: it sits under Audio
		// and is the only reliable machine signal separating an audiobook from
		// music (ARCHITECTURE.md §8.5). A bucket that merged them would destroy it.
		expect(tree[1].children.map((c) => c.id)).toEqual([3030]);
	});

	it('orders by id, which is the standard’s own order and therefore stable', () => {
		const tree = categoryTree([indexer({ categories: [audio, movies] })]);
		expect(tree.map((c) => c.id)).toEqual([2000, 3000]);
		expect(tree[0].children.map((c) => c.id)).toEqual([2030, 2040]);
	});

	it('unions across indexers without duplicating a shared parent', () => {
		const tree = categoryTree([
			indexer({ indexerId: 1, categories: [{ id: 2000, name: 'Movies', children: [] }] }),
			indexer({ indexerId: 2, categories: [movies] })
		]);
		expect(tree).toHaveLength(1);
		expect(tree[0].children.map((c) => c.id)).toEqual([2030, 2040]);
	});

	it('narrows to the selected indexers, which is what makes one tracker plus one category short', () => {
		const tree = categoryTree(
			[
				indexer({ indexerId: 1, categories: [movies] }),
				indexer({ indexerId: 2, categories: [audio] })
			],
			[2]
		);
		expect(tree.map((c) => c.id)).toEqual([3000]);
	});

	it('offers only what a search would actually ask', () => {
		// A disabled indexer's categories would populate the picker with choices
		// no leg is ever planned for.
		const tree = categoryTree([indexer({ enabled: false, categories: [movies] })]);
		expect(tree).toEqual([]);
	});

	it('keeps an unnamed category rather than dropping it', () => {
		// A category with no name is still a real filter; dropping it would
		// silently narrow what can be picked.
		const tree = categoryTree([indexer({ categories: [{ id: 8000, name: '', children: [] }] })]);
		expect(tree.map((c) => c.id)).toEqual([8000]);
		expect(categoryLabelFor(tree[0])).toBe('Category 8000');
	});

	it('prefers the first name it is given when two indexers disagree', () => {
		const tree = categoryTree([
			indexer({ indexerId: 1, categories: [{ id: 2000, name: '', children: [] }] }),
			indexer({ indexerId: 2, categories: [{ id: 2000, name: 'Movies', children: [] }] })
		]);
		expect(tree[0].name).toBe('Movies');
	});

	it('renders an upstream name verbatim, never a prettier re-spelling', () => {
		expect(categoryLabelFor({ id: 2040, name: 'Movies/HD' })).toBe('Movies/HD');
	});

	it('lets the catalogue win over a leftover learned name', () => {
		// 🔍 The order is the point. The learned union is what SSE frames named and
		// it only ever grows, so it can hold an indexer that has since been renamed
		// or deleted upstream; the catalogue is the authoritative list.
		const map = indexerNames(
			[indexer({ indexerId: 7, name: 'Renamed' })],
			[
				{ id: 7, name: 'Old name' },
				{ id: 8, name: 'Gone from Prowlarr' }
			]
		);
		expect(map.get(7)).toBe('Renamed');
		// The leftover is still resolvable, which is what lets the scope sentence
		// describe — and therefore let the user clear — a stale sticky selection.
		expect(map.get(8)).toBe('Gone from Prowlarr');
	});

	it('resolves every id in the tree, parents and leaves alike', () => {
		const names = categoryNames(categoryTree([indexer({ categories: [movies] })]));
		expect(names.get(2000)).toBe('Movies');
		expect(names.get(2040)).toBe('Movies/HD');
		expect(names.get(9999)).toBeUndefined();
	});
});

describe('catalogGuidance', () => {
	it('offers Services for every state UsArr’s own configuration causes', () => {
		for (const status of [
			CATALOG_NOT_CONFIGURED,
			CATALOG_NEVER_FETCHED,
			CATALOG_SERVICE_DISABLED
		]) {
			expect(catalogGuidance(status).fixInServices).toBe(true);
		}
	});

	it('offers no control for “answered and has none”, because the fix is not here', () => {
		// 🔍 The one state whose action is inside the indexer service. Services
		// would correctly show the connection as healthy, so a button pointing at
		// it is precisely the fake action §17.5 bans.
		expect(catalogGuidance(CATALOG_EMPTY).fixInServices).toBe(false);
	});

	it('offers nothing when the catalogue is fine', () => {
		expect(catalogGuidance(CATALOG_OK).fixInServices).toBe(false);
	});
});

describe('scopeSummary', () => {
	const ix = (id: number, name: string) => ({ id, name });

	it('says nothing at all when the scope is the default', () => {
		// A line that says "all indexers" on every search has taught the reader to
		// skip it by the time it matters.
		expect(scopeSummary({ indexers: [], categories: [], knownIndexers: 9 })).toBe('');
	});

	it('names one indexer', () => {
		expect(scopeSummary({ indexers: [ix(1, 'NZBgeek')], categories: [], knownIndexers: 9 })).toBe(
			'Searching NZBgeek only.'
		);
	});

	it('names up to three, alphabetically, so the sentence does not reshuffle', () => {
		expect(
			scopeSummary({
				indexers: [ix(1, 'Zeta'), ix(2, 'Alpha')],
				categories: [],
				knownIndexers: 9
			})
		).toBe('Searching Alpha, Zeta only.');
	});

	it('counts past three, where the proportion is the useful fact', () => {
		expect(
			scopeSummary({
				indexers: [ix(1, 'A'), ix(2, 'B'), ix(3, 'C'), ix(4, 'D')],
				categories: [],
				knownIndexers: 9
			})
		).toBe('Searching 4 of 9 indexers.');
	});

	it('never claims a selection larger than the catalogue it counts against', () => {
		// A stored selection can name indexers the catalogue no longer has, and
		// "5 of 2 indexers" would read as a bug in the screen.
		expect(
			scopeSummary({
				indexers: [ix(1, 'A'), ix(2, 'B'), ix(3, 'C'), ix(4, 'D')],
				categories: [],
				knownIndexers: 2
			})
		).toBe('Searching 4 of 4 indexers.');
	});

	it('always carries the noun on the category half', () => {
		// 🔍 "Searching Movies." is indistinguishable from a report of the query
		// somebody typed, and this line's job is to be legible as a statement
		// about the FILTER to a user who has forgotten they set one.
		expect(scopeSummary({ indexers: [], categories: [ix(2000, 'Movies')], knownIndexers: 9 })).toBe(
			'Searching in the Movies category.'
		);
		expect(
			scopeSummary({
				indexers: [],
				categories: [ix(5000, 'TV'), ix(2000, 'Movies')],
				knownIndexers: 9
			})
		).toBe('Searching in the Movies and TV categories.');
	});

	it('states both halves when both are set', () => {
		expect(
			scopeSummary({
				indexers: [ix(1, 'NZBgeek')],
				categories: [ix(2000, 'Movies')],
				knownIndexers: 9
			})
		).toBe('Searching NZBgeek only, in the Movies category.');
	});

	it('supplies the noun itself, so a caller must not supply one too', () => {
		// 🔍 The live bug this pins. A sticky selection outlives the catalogue that
		// named it, so the caller falls back to the bare id — and a fallback of
		// "category 2000" rendered "in the category 2000 category".
		expect(scopeSummary({ indexers: [], categories: [ix(2000, '2000')], knownIndexers: 0 })).toBe(
			'Searching in the 2000 category.'
		);
	});

	it('counts categories past three too', () => {
		expect(
			scopeSummary({
				indexers: [],
				categories: [ix(1, 'A'), ix(2, 'B'), ix(3, 'C'), ix(4, 'D')],
				knownIndexers: 9
			})
		).toBe('Searching in 4 categories.');
	});

	it('never uses a word §17.5 bans from this screen', () => {
		// The scope sentence renders on the same screen as the Grab button, so the
		// words that are a claim about a download are a claim wherever they appear.
		const line = scopeSummary({
			indexers: [ix(1, 'NZBgeek')],
			categories: [ix(2000, 'Movies')],
			knownIndexers: 9
		});
		for (const word of ['succeeded', 'success', 'downloading', 'done', 'retry']) {
			expect(line.toLowerCase()).not.toContain(word);
		}
	});
});
