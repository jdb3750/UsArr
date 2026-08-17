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
 *
 * ══════════════════════════════════════════════════════════════════════════
 * ⚠️⚠️ ONE SEARCH ASKS ONE INDEXER SERVICE, AND THE SCREEN SAYS WHICH ONE.
 * ══════════════════════════════════════════════════════════════════════════
 *
 * THE FACT THAT FORCES THIS. Two indexer services are creatable — `POST
 * /api/v1/services` answers 201 twice and `service_instance`'s only uniqueness
 * constraint is the name — but `GET /api/v1/search` takes `?instance=`, a
 * SINGLE service id, and `resolveIndexerInstance`
 * (internal/httpapi/search.go) resolves exactly one instance per search:
 * the one named, or `candidates[0]` by priority-then-name when none is. There
 * is no wire shape for a fan-out across two Prowlarrs.
 *
 * WHAT THE SCREEN USED TO DO WITH THAT, AND WHY IT WAS THE WORST OPTION. The
 * picker drew BOTH services' indexers in one flat grid with no label, keyed
 * the selection on the indexer id alone — so ticking a row ticked its twin in
 * the other service, because Prowlarr numbers its indexers per install and
 * both carry an id 3 — and then sent no `?instance=` at all. The search
 * contacted one service and never the other, under a note reading "Selecting
 * none searches all of them". A screen that appears to cover everything while
 * silently skipping a whole service is worse than one that covers less and
 * says so: the results look right.
 *
 * THE DECISION. The picker is scoped to ONE service at a time, chosen
 * explicitly, and `?instance=` carries that choice to the server. A selection
 * spanning two services is therefore not merely discouraged, it is
 * UNREPRESENTABLE — the grid only ever holds one service's rows, so there is
 * no state the request cannot express.
 *
 * THE TWO ALTERNATIVES, AND WHY NOT. (a) Keep one flat grid and fan out
 * client-side — two `startSearch` calls, two SSE streams, one merged table,
 * two closing reports to reconcile into one "n of m answered". That is a
 * subsystem, and CLAUDE.md's "cut before you add" is explicit that a proposal
 * which adds one must say what it removes. (b) Keep one flat grid and let
 * ticking a row in service B silently drop the ticks in service A. Same
 * unrepresentability, reached by a control that quietly undoes the user's last
 * action — surprise for the same correctness.
 *
 * WHERE THE SERVICE LABEL GOES. On the FIELDSET, once, not on every row —
 * `indexerPickerLegend` below. Every row in the grid belongs to the same
 * service by construction, and DESIGN-DIRECTION §9.1 is explicit that a value
 * identical across every row of a group is stated once and dropped from the
 * rows. That is the same rule `PRIORITY_NOTE` already obeys.
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

/* ── 0. Which indexer service this search asks ─────────────────────────────── */

/**
 * The indexer services the picker may choose between.
 *
 * Straight off the catalogue: `internal/httpapi/indexers.go` already filters to
 * `Role == "indexer"`, and it lists a DISABLED service rather than hiding one —
 * "it is off" is the most useful thing a list can say about it. So this is a
 * rename with a documented contract rather than a filter, and it exists so the
 * page never has to remember that `catalog.instances` is already role-scoped.
 *
 * ORDER IS THE CATALOGUE'S, AND IT IS THE SEARCH PATH'S TOO. Both read the one
 * `listServiceInstances` statement — `ORDER BY priority DESC, name ASC`, in
 * `store/serviceinstance.go`, the only ORDER BY over `service_instance` in the
 * tree — `ReadIndexerCatalog` through the shared body, `resolveIndexerInstance`
 * through the exported wrapper. The `name ASC` in `store/indexers.go` is
 * `indexerListSQL`, which orders indexers INSIDE one instance; this comment
 * once read it as the instance order and drew the wrong divergence from it.
 * (`service_instance.priority` is highest-wins, the opposite of the per-indexer
 * priority in `PRIORITY_NOTE`, which is what made that misreading easy.)
 *
 * ⚠️ WHAT DIVERGES IS MEMBERSHIP, NOT SORT. The catalogue keeps a DISABLED
 * service as a row, deliberately — see above. `resolveIndexerInstance` drops
 * disabled instances from its candidates. So `instances[0]` still is not
 * necessarily what an un-parameterised search hits: when the top row is turned
 * off, the search falls to the next enabled one down.
 *
 * Which is exactly why the client names the instance instead of leaning on
 * position. Anything here that pre-selects a radio is a CLIENT default the
 * screen then states in words; it is not a prediction of what an
 * un-parameterised search would have hit, and writing it as one would be the
 * same silent-agreement assumption that produced the bug.
 */
export function indexerServices(catalog: IndexerCatalog | undefined): CatalogInstance[] {
	return catalog?.instances ?? [];
}

/**
 * Which service the picker is showing, given what the user last chose.
 *
 * A remembered choice wins whenever the service is still there AND still
 * enabled. Otherwise the first ENABLED service, because a disabled one cannot
 * be searched at all: `resolveIndexerInstance` answers 409 `service_disabled`
 * for a named-but-disabled instance rather than quietly searching another, and
 * defaulting onto one would open the screen on a guaranteed error.
 *
 * 0 when there is nothing enabled to pick — every service off, or none
 * configured. The caller then sends no `?instance=` and the server's own
 * `no_indexer_service` / `service_disabled` answer is what the user sees, which
 * is the honest one rather than a client-invented sentence.
 */
export function resolveActiveInstance(
	instances: readonly CatalogInstance[],
	preferred: number
): number {
	const kept = instances.find((i) => i.instanceId === preferred && i.enabled);
	if (kept) return kept.instanceId;
	return instances.find((i) => i.enabled)?.instanceId ?? 0;
}

/** One service's indexers, and only that service's. The grid is built from
 * this, which is what makes a cross-service selection unrepresentable. */
export function indexersForInstance(
	indexers: readonly CatalogIndexer[],
	instanceId: number
): CatalogIndexer[] {
	return indexers.filter((i) => i.instanceId === instanceId);
}

/**
 * What a service's radio says about itself, under its name.
 *
 * It carries the state the picker would otherwise hide by scoping away from it:
 * with two services configured and the grid showing one, a service that UsArr
 * has never read, or that answered with nothing, or that the user switched off,
 * has no row anywhere else to say so. Facts, in the same voice as
 * `unavailableReason`, never a scolding.
 */
export function serviceFacts(instance: CatalogInstance): string {
	if (!instance.enabled) return 'turned off in UsArr, so a search cannot ask it';
	if (instance.fetchedAt === undefined || instance.fetchedAt === '') {
		return 'UsArr has not read its indexer list yet';
	}
	if (instance.indexerCount === 0) return 'no indexers configured in it';
	return `${instance.indexerCount} ${instance.indexerCount === 1 ? 'indexer' : 'indexers'}`;
}

/** Whether a search may name this service. Mirrors `resolveIndexerInstance`'s
 * own refusal, so the control is genuinely disabled rather than merely styled:
 * a radio that could only ever produce a 409 is a control that does not work. */
export function isServiceSelectable(instance: CatalogInstance): boolean {
	return instance.enabled;
}

/**
 * The indexer fieldset's legend, which is where the service label lives.
 *
 * ONCE, ON THE GROUP. Every row below it belongs to the named service by
 * construction, and §9.1 states a value identical across every row of a group
 * once rather than repeating it — the rule `PRIORITY_NOTE` already follows. The
 * name is only added when there is a choice to disambiguate: on the one-service
 * install, which is most of them, naming it is noise.
 */
export function indexerPickerLegend(serviceName: string, serviceCount: number): string {
	if (serviceCount <= 1 || serviceName === '') return 'Indexers to search';
	return `Indexers to search in ${quoted(serviceName)}`;
}

/**
 * The note under the grid: what an empty selection means, stated for the
 * install this actually is.
 *
 * ⚠️ THE SENTENCE THIS REPLACES WAS FALSE. It read "Selecting none searches all
 * of them" beside a list drawn from two services, while the search asked one.
 * With one service configured it was true and is kept verbatim; with more it
 * names the boundary the request itself has.
 */
export function pickerScopeNote(serviceCount: number): string {
	if (serviceCount <= 1) return 'Selecting none searches all of them.';
	return (
		'Selecting none searches every indexer in this service. One search asks one indexer ' +
		'service, so the others are not asked — pick a different one above to search it.'
	);
}

/**
 * The label on the control that returns to an unscoped search.
 *
 * "Search all indexers and categories" is a promise the request cannot keep
 * once a second service exists, and this control is the one a user reaches for
 * precisely when they suspect the screen is hiding results from them. It is
 * rendered in three places on Requests and they must not drift apart, which is
 * why it is one function rather than three literals.
 */
export function clearScopeLabel(serviceCount: number): string {
	if (serviceCount <= 1) return 'Search all indexers and categories';
	return 'Search every indexer and category in this service';
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
 *
 * ⚠️ IT IS A UNION, AND THE UNION FORGETS WHOSE WORDING IT KEPT. Where two
 * indexers name 2000 differently the first non-empty name wins — first in the
 * order the indexers arrive in — and the rest are discarded, with nothing left
 * recording which indexer supplied the survivor. So a PER-INDEXER label must
 * never be resolved from this tree: a release row's category tooltip joined
 * against it would populate on every row and look entirely right, while showing
 * one indexer's wording for ANOTHER indexer's category — silently destroying the
 * divergence that is the whole reason that tooltip carries the raw category path
 * at all. It has to join against that row's own indexer's subtree.
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
 * ⚠️ THE PER-INDEXER JOIN — WHAT `categoryTree` ABOVE IS STRUCTURALLY UNABLE TO
 * ANSWER, AND THE ONLY HONEST WAY TO ASK IT.
 *
 * A release row's Category cell shows UsArr's OWN derived words —
 * `$lib/requests.categoryLabel` reads the server's `type:` / `format:` tags, one
 * vocabulary across every indexer, which is what makes the column comparable
 * down its length. The tooltip is the other half: what the indexer that actually
 * returned this release calls the categories it filed it under. Prowlarr's
 * `Movies/HD` against another tracker's `HD Movies` for the same 2040 is
 * ORDINARY, and that divergence is the entire reason the tooltip exists.
 *
 * ⚠️ SO IT JOINS AGAINST ONE INDEXER'S OWN SUBTREE, NEVER THE MERGED TREE.
 * `categoryTree` is a UNION whose name collisions are settled first-non-empty-
 * wins in indexer arrival order, and `CatalogCategory` keeps no record of which
 * indexer supplied the survivor — see that function's own warning. A tooltip
 * resolved from it would populate on EVERY row and look entirely right while
 * printing one indexer's wording under another indexer's category, destroying
 * the divergence it was added to show. There is no way to detect that at the
 * call site, which is why the lookup is here rather than over a shared map.
 *
 * ⚠️ THE KEY IS THE PAIR, for the reason `IndexerPair` below spells out at
 * length: Prowlarr numbers its indexers per install, so a bare indexer id 3
 * names a different tracker in each configured service. The caller passes the
 * instance the SEARCH was resolved to — the server's own `instance_id` off the
 * accepted body — and not whatever the picker happens to be showing now, which
 * the user may have switched since the results landed. Neither wrong key fails
 * loudly, which is the trap: ids are small and dense per install, so the other
 * instance almost certainly HAS an indexer 3, and the lookup finds it and prints
 * that tracker's wording rather than falling through to the empty string below.
 *
 * ⚠️ IT DEGRADES TO THE EMPTY STRING, AND THE CALLER RENDERS NO TOOLTIP RATHER
 * THAN AN EMPTY ONE. Four absences reach it and none is an error: no instance
 * resolved yet, a release whose frame carried no `indexer_id`, an indexer the
 * catalogue replica has not got (deleted upstream, or `never_fetched`), and an
 * indexer that names none of the ids on this release. A missing tooltip is a
 * fact about the replica; a guessed one would be the bug above under a new name.
 *
 * The walk is recursive because `api.toCatalogIndexer` builds the tree to
 * whatever depth the wire has — `categoryTree` reads two levels because that is
 * what the PICKER draws, which is not a claim about the data.
 */
export function indexerCategoryTitle(
	indexers: readonly CatalogIndexer[],
	instanceId: number,
	indexerId: number | undefined,
	categoryIds: readonly number[]
): string {
	if (!Number.isSafeInteger(instanceId) || instanceId <= 0) return '';
	if (indexerId === undefined || !Number.isSafeInteger(indexerId) || indexerId < 0) return '';
	const source = indexers.find((i) => i.instanceId === instanceId && i.indexerId === indexerId);
	if (source === undefined) return '';

	const named = new Map<number, string>();
	const walk = (nodes: readonly CatalogCategory[]): void => {
		for (const node of nodes) {
			// Unnamed ids are skipped rather than filled in with `Category 2040`:
			// the cell already falls back to the bare ids, so a tooltip repeating
			// them adds nothing and would make "this indexer has no wording for it"
			// indistinguishable from "it does".
			if (node.name !== '' && !named.has(node.id)) named.set(node.id, node.name);
			walk(node.children);
		}
	};
	walk(source.categories);

	// The release's own order, which is Prowlarr's — parent first, so `[3000,
	// 3030]` reads `Audio · Audio/Audiobook` down the same path the release was
	// filed along. Repeats are dropped so a doubled id cannot print twice.
	const out: string[] = [];
	for (const id of categoryIds) {
		const name = named.get(id);
		if (name === undefined || out.includes(name)) continue;
		out.push(name);
	}
	return out.join(' · ');
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
	/** The service this search will ask, and how many are configured. Omitted
	 * where the catalogue has not been read, which is the only state in which
	 * the screen genuinely does not know. */
	service?: { name: string; total: number };
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
 *
 * ⚠️ WITH A SECOND INDEXER SERVICE CONFIGURED IT IS NEVER EMPTY, and that is
 * the correctness half rather than a flourish. The empty string exists because
 * "all indexers" on every search teaches the reader to skip the line by the
 * time it matters — but once there are two services, the default scope is NOT
 * "all indexers", it is "every indexer in one of your two services". That is a
 * scope the user did not set and would not guess, so it is the one that most
 * needs saying, and the line states which service and how many are left out.
 */
export function scopeSummary(names: ScopeNames): string {
	const parts: string[] = [];
	if (names.indexers.length > 0) parts.push(namedScope(names.indexers, names.knownIndexers));
	if (names.categories.length > 0) parts.push(`in ${namedCategories(names.categories)}`);

	const service = names.service;
	if (service === undefined || service.total <= 1) {
		if (parts.length === 0) return '';
		return `Searching ${parts.join(', ')}.`;
	}

	// The name is rendered upstream data — the user's own label for the service
	// — so it is quoted rather than re-spelled, exactly as the indexer names on
	// the rows are. §17.5's copy rules govern UsArr's sentences, never this.
	const scope = parts.length === 0 ? 'every indexer' : parts.join(', ');
	const others = service.total - 1;
	const rest =
		others === 1
			? 'The other indexer service is not asked.'
			: `The other ${others} indexer services are not asked.`;
	return `Searching in ${quoted(service.name)}: ${scope}. ${rest}`;
}

/** Upstream data, set off from UsArr's own words. One helper so the quote
 * characters cannot drift between the legend and the scope line. */
function quoted(name: string): string {
	return `“${name}”`;
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

/* ── 5. The shape a selection is stored and read in ────────────────────────── */

/**
 * ⚠️ AN INDEXER ID IS ONLY MEANINGFUL BESIDE ITS SERVICE'S ID.
 *
 * Prowlarr numbers its indexers per install, so two configured services each
 * carry an id 1, an id 2 and an id 3, naming entirely different trackers. A
 * selection stored as bare indexer ids is therefore not a selection at all —
 * it is a set of numbers whose meaning depends on which service happens to be
 * active when it is read, which is what made ticking one row tick its twin.
 *
 * So the unit is the PAIR, from the checkbox through localStorage to the
 * `?indexer=` list, and there is no point on the path where the instance is
 * dropped and reconstructed.
 *
 * The wire form is `instance:indexer`, comma-separated — the same
 * `${instanceId}:${indexerId}` the picker's `each` block has always keyed on,
 * so one reader can hold both in their head. Parsing is total: anything
 * unparseable is dropped rather than thrown, because a corrupt preference must
 * not be able to take the screen down.
 */
export interface IndexerPair {
	instanceId: number;
	indexerId: number;
}

export function parsePairs(raw: string | null): IndexerPair[] {
	if (!raw) return [];
	const out: IndexerPair[] = [];
	for (const part of raw.split(',')) {
		const [left, right, ...rest] = part.split(':');
		// A two-field form and nothing else. A bare `3` is the RETIRED shape and
		// is refused here rather than read as `0:3` — see `RETIRED_KEYS` in
		// `$lib/indexerscope.svelte` for why reinterpreting it was rejected.
		if (right === undefined || rest.length > 0) continue;
		const instanceId = Number.parseInt(left, 10);
		const indexerId = Number.parseInt(right, 10);
		if (!Number.isSafeInteger(instanceId) || instanceId <= 0) continue;
		if (!Number.isSafeInteger(indexerId) || indexerId < 0) continue;
		if (out.some((p) => p.instanceId === instanceId && p.indexerId === indexerId)) continue;
		out.push({ instanceId, indexerId });
	}
	return out;
}

export function formatPairs(pairs: readonly IndexerPair[]): string {
	return pairs.map((p) => `${p.instanceId}:${p.indexerId}`).join(',');
}

/** The indexer ids selected within ONE service — which is what a search sends,
 * because a search asks one service. */
export function pairsForInstance(pairs: readonly IndexerPair[], instanceId: number): number[] {
	if (instanceId <= 0) return [];
	return pairs.filter((p) => p.instanceId === instanceId).map((p) => p.indexerId);
}
