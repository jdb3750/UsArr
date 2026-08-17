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
	clearScopeLabel,
	formatPairs,
	indexerCategoryTitle,
	indexerFacts,
	indexerNames,
	indexerPickerLegend,
	indexerServices,
	indexersForInstance,
	isSearchable,
	isServiceSelectable,
	pairsForInstance,
	parsePairs,
	pickerScopeNote,
	PRIORITY_NOTE,
	resolveActiveInstance,
	scopeSummary,
	serviceFacts,
	sortIndexers,
	unavailableReason,
	type CatalogIndexer,
	type CatalogInstance
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

describe('a release row’s own indexer’s wording for its categories', () => {
	const movies = {
		id: 2000,
		name: 'Movies',
		children: [{ id: 2040, name: 'Movies/HD', children: [] }]
	};

	it('gives each row its OWN indexer’s wording when two indexers disagree', () => {
		// 🔍 THIS IS THE TEST THE FEATURE EXISTS FOR, and it is the one case that
		// can tell the correct join apart from a lookup over `categoryTree`'s
		// union. Both indexers advertise 2040; they name it differently, which is
		// ordinary. The union resolves 2040 to ONE of these by arrival order and
		// keeps no record of whose it kept, so a tooltip built on it would print
		// the same string on both rows and look entirely right.
		const catalogue = [
			indexer({
				indexerId: 1,
				categories: [{ id: 2040, name: 'Movies/HD', children: [] }]
			}),
			indexer({
				indexerId: 2,
				categories: [{ id: 2040, name: 'HD Movies', children: [] }]
			})
		];
		expect(indexerCategoryTitle(catalogue, 1, 1, [2040])).toBe('Movies/HD');
		expect(indexerCategoryTitle(catalogue, 1, 2, [2040])).toBe('HD Movies');
		// And the union really is the thing being avoided: it answers with one
		// wording for the id, whichever row asked.
		expect(categoryTree(catalogue).map((c) => c.name)).toEqual(['Movies/HD']);
	});

	it('reads the pair, so an id 3 in one service never answers for an id 3 in another', () => {
		// Prowlarr numbers its indexers per install — the same fact `IndexerPair`
		// exists for. Instance 2's indexer 1 is a different tracker.
		const catalogue = [
			indexer({
				instanceId: 1,
				indexerId: 1,
				categories: [{ id: 2040, name: 'Movies/HD', children: [] }]
			}),
			indexer({
				instanceId: 2,
				indexerId: 1,
				categories: [{ id: 2040, name: 'HD Movies', children: [] }]
			})
		];
		expect(indexerCategoryTitle(catalogue, 1, 1, [2040])).toBe('Movies/HD');
		expect(indexerCategoryTitle(catalogue, 2, 1, [2040])).toBe('HD Movies');
	});

	it('walks into the children, where the ids that carry the signal live', () => {
		// 3030 under Audio is the audiobook signal; a lookup that only read the
		// parents would have nothing to say about the one id that matters most.
		const audio = {
			id: 3000,
			name: 'Audio',
			children: [{ id: 3030, name: 'Audio/Audiobook', children: [] }]
		};
		expect(indexerCategoryTitle([indexer({ categories: [audio] })], 1, 1, [3000, 3030])).toBe(
			'Audio · Audio/Audiobook'
		);
	});

	it('keeps the release’s own order and prints a repeated name once', () => {
		const catalogue = [indexer({ categories: [movies] })];
		expect(indexerCategoryTitle(catalogue, 1, 1, [2040, 2000])).toBe('Movies/HD · Movies');
		expect(indexerCategoryTitle(catalogue, 1, 1, [2000, 2000])).toBe('Movies');
	});

	it('says nothing when the row’s indexer is not in the catalogue', () => {
		const catalogue = [indexer({ indexerId: 1, categories: [movies] })];
		// Deleted upstream, or a `never_fetched` replica. Both are ordinary.
		expect(indexerCategoryTitle(catalogue, 1, 9, [2000])).toBe('');
		expect(indexerCategoryTitle([], 1, 1, [2000])).toBe('');
	});

	it('says nothing when the row or the search names no id to join on', () => {
		const catalogue = [indexer({ categories: [movies] })];
		// A frame that carried no `indexer_id`, and a search whose instance is not
		// resolved yet. Neither is an error and neither may be guessed past.
		expect(indexerCategoryTitle(catalogue, 1, undefined, [2000])).toBe('');
		expect(indexerCategoryTitle(catalogue, 0, 1, [2000])).toBe('');
	});

	it('says nothing when this indexer has no wording for these ids', () => {
		const catalogue = [indexer({ categories: [movies] })];
		// An id the indexer does not advertise, and an id it advertises unnamed.
		expect(indexerCategoryTitle(catalogue, 1, 1, [7020])).toBe('');
		expect(indexerCategoryTitle(catalogue, 1, 1, [])).toBe('');
		expect(
			indexerCategoryTitle(
				[indexer({ categories: [{ id: 8000, name: '', children: [] }] })],
				1,
				1,
				[8000]
			)
		).toBe('');
	});

	it('answers for a disabled indexer, because a row from it is still on screen', () => {
		// 🔍 Deliberately NOT `isSearchable`. That gate belongs to the picker,
		// which offers what a FUTURE search could ask; this describes a release
		// that has already come back, and an indexer switched off since the search
		// ran still named the category it filed that release under.
		const off = indexer({ enabled: false, supportsSearch: false, categories: [movies] });
		expect(indexerCategoryTitle([off], 1, 1, [2000])).toBe('Movies');
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

/* ── One search, one indexer service ──────────────────────────────────────── */

function service(over: Partial<CatalogInstance> = {}): CatalogInstance {
	return {
		instanceId: 1,
		name: 'Prowlarr',
		kind: 'prowlarr',
		enabled: true,
		status: CATALOG_OK,
		message: '3 indexers from "Prowlarr"',
		action: '',
		fetchedAt: '2026-08-16T12:00:00Z',
		indexerCount: 3,
		...over
	};
}

describe('which indexer service a search asks', () => {
	// 🔍 THE BUG THIS BLOCK EXISTS FOR. Two indexer services are creatable, the
	// search endpoint takes ONE `?instance=`, and the client sent none — so
	// `resolveIndexerInstance` took `candidates[0]` and one service was never
	// contacted, under a picker that listed both services' indexers together.

	const alpha = service({ instanceId: 1, name: 'Alpha' });
	const beta = service({ instanceId: 2, name: 'Beta' });

	it('keeps the remembered service when it is still there and still on', () => {
		expect(resolveActiveInstance([alpha, beta], 2)).toBe(2);
	});

	it('falls back to the first enabled service when the remembered one is gone', () => {
		// A service deleted while a selection was sticky. Falling back rather than
		// resolving to nothing keeps the screen usable, and the scope line names
		// whichever it landed on so the change is not silent.
		expect(resolveActiveInstance([alpha, beta], 7)).toBe(1);
	});

	it('refuses to default onto a disabled service', () => {
		// `resolveIndexerInstance` answers 409 service_disabled for a
		// named-but-disabled instance rather than searching another, so opening on
		// one would open on a guaranteed error.
		const off = service({ instanceId: 1, name: 'Alpha', enabled: false });
		expect(resolveActiveInstance([off, beta], 1)).toBe(2);
		expect(resolveActiveInstance([off, beta], 0)).toBe(2);
	});

	it('resolves to nothing when every service is off, rather than picking one', () => {
		// The server's own no-indexer-service answer is the honest one here; a
		// client-invented instance id would turn it into a 409 about a service the
		// user did not choose.
		const offA = service({ instanceId: 1, enabled: false });
		const offB = service({ instanceId: 2, enabled: false });
		expect(resolveActiveInstance([offA, offB], 1)).toBe(0);
		expect(resolveActiveInstance([], 1)).toBe(0);
	});

	it('offers only the active service’s indexers, which is what makes the request expressible', () => {
		// 🔍 Both services carry an indexer id 3, because Prowlarr numbers them per
		// install. This filter is the whole reason a tick can no longer mean two
		// different trackers at once.
		const rows = [
			indexer({ instanceId: 1, indexerId: 3, name: 'Alpha 3' }),
			indexer({ instanceId: 2, indexerId: 3, name: 'Beta 3' }),
			indexer({ instanceId: 2, indexerId: 4, name: 'Beta 4' })
		];
		expect(indexersForInstance(rows, 2).map((i) => i.name)).toEqual(['Beta 3', 'Beta 4']);
		expect(indexersForInstance(rows, 1).map((i) => i.name)).toEqual(['Alpha 3']);
		expect(indexersForInstance(rows, 0)).toEqual([]);
	});

	it('says on each service’s own option what scoping away from it would hide', () => {
		expect(serviceFacts(service({ indexerCount: 3 }))).toBe('3 indexers');
		expect(serviceFacts(service({ indexerCount: 1 }))).toBe('1 indexer');
		expect(serviceFacts(service({ indexerCount: 0 }))).toBe('no indexers configured in it');
		expect(serviceFacts(service({ fetchedAt: undefined, indexerCount: 0 }))).toBe(
			'UsArr has not read its indexer list yet'
		);
		// The disabled arm outranks the rest: it is the reason the option cannot
		// be picked, and the count is not what the reader needs.
		expect(serviceFacts(service({ enabled: false, indexerCount: 3 }))).toBe(
			'turned off in UsArr, so a search cannot ask it'
		);
	});

	it('lets a search name only a service it could actually ask', () => {
		expect(isServiceSelectable(service())).toBe(true);
		expect(isServiceSelectable(service({ enabled: false }))).toBe(false);
	});

	it('takes the catalogue’s instances as the services, in the server’s order', () => {
		// indexers.go already filters to Role == "indexer", so this passes the
		// list through rather than re-filtering it. The order is the server's and
		// is deliberately NOT re-sorted here: catalogue and search path share one
		// `ORDER BY priority DESC, name ASC`, so they AGREE on sort. What they
		// disagree on is membership — the catalogue keeps disabled services,
		// resolveIndexerInstance drops them — which is why the client names the
		// instance rather than trusting position.
		const catalog = {
			status: CATALOG_OK,
			message: '',
			action: '',
			instances: [beta, alpha],
			indexers: []
		};
		expect(indexerServices(catalog).map((s) => s.instanceId)).toEqual([2, 1]);
		expect(indexerServices(undefined)).toEqual([]);
	});
});

describe('the copy that changes meaning with a second service', () => {
	it('names the service on the group rather than on every row', () => {
		// §9.1: a value identical across every row of a group is stated once. Every
		// row in the grid belongs to the named service by construction.
		expect(indexerPickerLegend('Alpha', 2)).toBe('Indexers to search in “Alpha”');
	});

	it('does not name a choice the one-service install does not have', () => {
		expect(indexerPickerLegend('Alpha', 1)).toBe('Indexers to search');
		expect(indexerPickerLegend('', 2)).toBe('Indexers to search');
	});

	it('keeps “searches all of them” only where it is true', () => {
		// 🔍 THE FALSE SENTENCE. It sat under a grid drawn from two services while
		// the search asked one of them.
		expect(pickerScopeNote(1)).toBe('Selecting none searches all of them.');
		expect(pickerScopeNote(2)).not.toContain('searches all of them');
		expect(pickerScopeNote(2)).toContain('every indexer in this service');
		expect(pickerScopeNote(2)).toContain('the others are not asked');
	});

	it('stops the clear control promising a coverage the request cannot carry', () => {
		expect(clearScopeLabel(1)).toBe('Search all indexers and categories');
		expect(clearScopeLabel(2)).toBe('Search every indexer and category in this service');
	});
});

describe('the scope sentence with more than one indexer service', () => {
	// A resolved name, as the page hands them over. Local to this block: the
	// helper of the same name in the one-service block above is scoped to it.
	const ix = (id: number, name: string) => ({ id, name });

	it('is never silent, because the default scope is not one anybody would guess', () => {
		// 🔍 The empty string exists so "all indexers" on every search does not
		// train the reader to skip the line. With two services the default is
		// "every indexer in one of your two", which is exactly the scope that most
		// needs saying — and the one the screen used to describe as everything.
		expect(
			scopeSummary({
				indexers: [],
				categories: [],
				knownIndexers: 6,
				service: { name: 'Alpha', total: 2 }
			})
		).toBe('Searching in “Alpha”: every indexer. The other indexer service is not asked.');
	});

	it('stays silent on the default scope when there is only one service', () => {
		expect(
			scopeSummary({
				indexers: [],
				categories: [],
				knownIndexers: 3,
				service: { name: 'Alpha', total: 1 }
			})
		).toBe('');
	});

	it('counts the services it is not asking', () => {
		expect(
			scopeSummary({
				indexers: [],
				categories: [],
				knownIndexers: 6,
				service: { name: 'Alpha', total: 3 }
			})
		).toBe('Searching in “Alpha”: every indexer. The other 2 indexer services are not asked.');
	});

	it('carries the service beside both halves of a narrowed scope', () => {
		expect(
			scopeSummary({
				indexers: [ix(1, 'NZBgeek')],
				categories: [ix(2000, 'Movies')],
				knownIndexers: 6,
				service: { name: 'Alpha', total: 2 }
			})
		).toBe(
			'Searching in “Alpha”: NZBgeek only, in the Movies category. ' +
				'The other indexer service is not asked.'
		);
	});

	it('leaves the one-service sentence exactly as it was', () => {
		// The common install must not pay for the two-service fix in extra words.
		const bare = scopeSummary({ indexers: [ix(1, 'NZBgeek')], categories: [], knownIndexers: 9 });
		expect(bare).toBe('Searching NZBgeek only.');
		expect(
			scopeSummary({
				indexers: [ix(1, 'NZBgeek')],
				categories: [],
				knownIndexers: 9,
				service: { name: 'Alpha', total: 1 }
			})
		).toBe(bare);
	});
});

describe('the stored selection’s shape', () => {
	// 🔍 An indexer id is only meaningful beside its service's id: Prowlarr
	// numbers its indexers per install, so two services each carry an id 3.

	it('round-trips a pair', () => {
		expect(formatPairs(parsePairs('1:3,2:3,2:4'))).toBe('1:3,2:3,2:4');
	});

	it('refuses the retired bare-id shape rather than reading it as instance 0', () => {
		// 🔍 THE MIGRATION DECISION, PINNED. `usarr.search.indexers` held `3,5`,
		// and adopting those under whichever service happens to be active is
		// reading a stored value under a meaning it never had — the exact class of
		// bug this shape closes. They are dropped, and `RETIRED_KEYS` deletes them.
		expect(parsePairs('3,5')).toEqual([]);
		expect(parsePairs('1:3,5')).toEqual([{ instanceId: 1, indexerId: 3 }]);
	});

	it('drops anything unparseable instead of throwing', () => {
		// A corrupt preference must not be able to take the screen down.
		expect(parsePairs(null)).toEqual([]);
		expect(parsePairs('')).toEqual([]);
		expect(parsePairs('a:b,1:,:3,0:3,-1:2,1:2:3')).toEqual([]);
		expect(parsePairs('1:-4')).toEqual([]);
	});

	it('does not store the same pair twice', () => {
		expect(parsePairs('1:3,1:3')).toEqual([{ instanceId: 1, indexerId: 3 }]);
	});

	it('sends one service’s ids and never the other’s', () => {
		// 🔍 This is the read the search uses. Two services both carrying an id 3
		// is ordinary, and only one of them may reach `?indexer=`.
		const pairs = parsePairs('1:3,2:3,2:7');
		expect(pairsForInstance(pairs, 2)).toEqual([3, 7]);
		expect(pairsForInstance(pairs, 1)).toEqual([3]);
		expect(pairsForInstance(pairs, 9)).toEqual([]);
		// Without an active service there is no meaningful answer, and guessing
		// one is the bug.
		expect(pairsForInstance(pairs, 0)).toEqual([]);
	});
});
