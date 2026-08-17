/**
 * HOME's vocabulary, as pure functions. ARCHITECTURE.md §17.2 (as amended by
 * ADR-0028), §17.7, and DESIGN-DIRECTION §8.4, §9.6, §10.
 *
 * DOM-free and deterministic, so the node-environment vitest run can pin it.
 * The rendering is in `routes/+page.svelte`; the decisions that §17.2 makes
 * RULES rather than taste live here, because a rule inside a `{#if}` in a
 * template is a rule nothing can test.
 *
 * WHICH OF HOME'S THREE BLOCKS THIS FILE SERVES, AND WHY IT IS ONLY ONE.
 *
 * ADR-0028 fixes Home at three blocks: A, a ≤6-row media-type summary; B, an
 * attention block hidden entirely when empty; C, one unified recently-added
 * table. ⚠️ THIS READ **"A and C have no data source in this build and are
 * therefore not drawn at all"**, on the ground that *"the `work` / `edition` /
 * `media_file` tables and the sync channels that would fill them are unbuilt"*.
 * **Half of that premise is dead.** A first-import channel exists for Kavita —
 * it fires when a Kavita client stack is built or on demand, with no periodic
 * re-sync behind it — and it writes those tables; `GET /api/v1/library/recent`
 * reads them. **So Block C IS drawn** (`routes/+page.svelte`, `library` mode).
 * **Block A is still not**, and it is worth being exact about which reason
 * survives, because the sweeping one did not: Block A's per-type rollup is a
 * SECOND read and nothing serves it, so every count in it would still have to
 * be invented. DESIGN-DIRECTION §9.6 closes that off in as many words: never
 * fabricated data in a shipped product surface. A zeroed table and a skeleton
 * are the same fabrication with different punctuation. Block B is what this
 * file computes, from GET /api/v1/services/health. It was the only block with a
 * real source on the day this was written; it is now one of two.
 *
 * ⚠️ TWO THINGS ON HOME ARE NOT ONE OF THE THREE BLOCKS, and saying so here is
 * the point rather than a caveat. A release-search entry point and the recent
 * grabs list both have real sources in this build and neither is Block A or
 * Block C. ⚠️ THE REASON GIVEN FOR THAT WAS **"`Recently added` is a catalogue
 * read that does not exist yet"**, AND THAT PREMISE IS DEAD WITH THE ONE ABOVE:
 * the read exists and Block C is drawn off it. **The conclusion does not move,
 * and it now rests on something firmer than what it was given.** While Block C
 * was undrawn, filing recent grabs under its heading was an IMPRECISION — a
 * claim about a catalogue that no catalogue on the screen could contradict.
 * Block C exists now, so that heading names a REGION THAT IS ALREADY THERE
 * MEANING SOMETHING ELSE: `Recently added` is the CATALOGUE ordered by
 * `added_at`, while a grab is a release handed to a download client, which
 * UsArr stops watching at the moment Prowlarr accepts it (`$lib/requests`'
 * KNOWLEDGE_STOPS_NOTE). The two claims would COLLIDE rather than merely
 * overstate — one form of words over two different facts, both on screen at
 * once — which is the invented status the rest of this file is written against.
 * What this module adds for them is the two rules that are rules — whether a
 * search box may be drawn at all (`hasIndexer`) and what it must say it
 * searches (`HOME_SEARCH_SCOPE_NOTE`) — because a rule inside a `{#if}` in a
 * template is a rule nothing can test. The grabs themselves are
 * `$lib/requests`' vocabulary, reused rather than restated.
 *
 * §17.2's hard rule points the same way and is worth restating because it is
 * the one an implementer breaks first: a media type the user does not have is
 * not shown AT ALL, not in Block A, not in the sidebar, not as a search group.
 * With no catalogue there are no types the user has, so the honest count of
 * media-type rows anywhere on this screen is zero. The sidebar already does
 * this (see `routes/+layout.svelte`); Home now matches it.
 *
 * WHAT MODE THE SCREEN IS IN IS DERIVED FROM THE API AND NEVER FROM A
 * CONSTANT. §8.5 defines Search-and-Grab mode as "activated when no configured
 * instance advertises `LibrarySync`", and the health endpoint carries the
 * `role` that decides it. So the mode is a function of the response, and a
 * build that later accepts a library-bearing kind changes what this returns
 * without anything here being edited.
 */

import type { ServiceHealth, ServicesHealth } from './api';
import {
	isIndexer,
	needsAttention,
	stateLabel,
	type ServiceRow,
	type StateLabel,
	type Tone
} from './services';

/**
 * Which state §17.7 puts Home in, decided from what is actually configured.
 *
 *   `unconfigured`     nothing is connected. §17.7: this goes to the first-run
 *                      path and NEVER to an empty home page.
 *   `search-and-grab`  services exist and none of them is library-bearing.
 *                      §8.5's named configuration, not an implicit empty app.
 *   `library`          at least one library-bearing service is configured.
 *
 * `library` BECAME REACHABLE when `kavita` → role `library` joined
 * `internal/httpapi.serviceKinds` (ADR-0041), and this comment used to say the
 * opposite — that every instance which could exist was an indexer. The mode was
 * derived rather than assumed away precisely so that this day needed no change
 * here, and it did not.
 *
 * What the arm renders is still one honest sentence and NO catalogue, and that
 * is still what is true of it: a Kavita can be added and probed, and nothing
 * imports from it yet. Read `internal/db/migrations` and `web/src/routes` for
 * what exists, never this comment.
 */
export type HomeMode = 'unconfigured' | 'search-and-grab' | 'library';

export function homeMode(health: ServicesHealth): HomeMode {
	if (health.setupRequired || health.services.length === 0) return 'unconfigured';
	// §8.5's own test, applied to the field that carries it. `isIndexer` is the
	// Services screen's existing predicate rather than a second copy of it.
	if (health.services.every(isIndexer)) return 'search-and-grab';
	return 'library';
}

/**
 * WHETHER HOME MAY OFFER A RELEASE SEARCH AT ALL, and it is deliberately NOT
 * `mode === 'search-and-grab'`.
 *
 * The precondition for the box is that something can answer it: at least one
 * configured service is an indexer. That is a narrower test than the mode and a
 * different one — `library` mode means a library-bearing service exists, and an
 * install with a Sonarr AND a Prowlarr is in `library` mode with a perfectly
 * good indexer to search. Gating on the mode would delete a working control on
 * the first build that connects a Sonarr, which is the same class of mistake
 * `homeMode` exists to avoid on the other side.
 *
 * The inverse matters more: on an `unconfigured` install this is false, so Home
 * never draws an input that has nothing behind it. A search box that accepts a
 * query and can only fail is the invented status CLAUDE.md forbids, expressed
 * as a control rather than as a number — the same reason `routes/search` draws
 * a sentence instead of an input over an index that does not exist.
 */
export function hasIndexer(health: ServicesHealth): boolean {
	return health.services.some(isIndexer);
}

/**
 * WHAT THE BOX SEARCHES, SAID ON THE BOX.
 *
 * DESIGN-DIRECTION §8.3 keeps two things apart that the *Arrs also keep apart:
 * **library search** — the persistent input, jumping to something you already
 * have — and **release search**, a dedicated screen. *"Merging them is how a
 * 0 ms local query ends up waiting on a 30 s indexer."* Home's box is the
 * second kind and must say so, because an unlabelled input on a home screen
 * reads as the first kind by default.
 *
 * It is also why the box is a ROUTE and not a search field. Nothing on Home
 * fans out to an indexer; submitting navigates to Requests with `?q=`, which is
 * §17.4's own mechanism (`requestsSearchHref`). Home stays a local read.
 *
 * ⚠️ THIS STRING IS THE SEAM, AND IT IS THE ONLY THING THAT CHANGES WHEN THE
 * LIBRARY INDEX LANDS. §8.3 gives that index a persistent input, at which point
 * Home's box becomes library search: same element, same submit, a different
 * destination and this sentence rewritten. Keeping the destination in
 * `requestsSearchHref` and the scope in a constant is what makes that a
 * two-line change rather than a rebuild. The seam ships; the feature does not.
 */
export const HOME_SEARCH_SCOPE_NOTE =
	'This searches releases on your indexers, not your own library. Results, the indexer and ' +
	'category pickers and the grab are on Requests, and pressing Search takes you there.';

/**
 * One row of Block B. §17.2: the block is the differentiator and no surveyed
 * tool has anything to put in it, and it is **hidden when empty** rather than
 * rendering a green "all good" panel.
 *
 * THE COLUMNS ARE NOT THE SERVICES SCREEN'S COLUMNS, and the difference is
 * §17.3's rule that a problem is stated canonically once per screen. `state`
 * and `detail` are UsArr's own words, straight from `stateLabel()` so the two
 * screens cannot drift; `problem` is the upstream's own words, verbatim, and
 * is the only thing here that is not UsArr speaking. What Block B does NOT
 * carry is the fix: the button that repairs a service lives on the Services
 * row, and Home links to that row instead of growing a second copy of it.
 */
export interface AttentionRow {
	id: number;
	name: string;
	tone: Tone;
	icon: StateLabel['icon'];
	/** UsArr's own plain-language word for the state. Never the mechanism's. */
	state: string;
	/** The qualifying clause under it, or ''. */
	detail: string;
	/** The upstream's own text, VERBATIM after redaction, or ''. */
	problem: string;
}

/**
 * Block B's rows, in the order the services came back in.
 *
 * The membership test is `needsAttention()`, which is the same predicate the
 * Services roll-up uses. Two lists answering "what is wrong" from two
 * predicates is how a screen ends up reporting three problems while another
 * reports four, and the deduction the user makes from that is that one of them
 * is lying.
 */
export function attention(services: readonly ServiceHealth[], now: Date): AttentionRow[] {
	const out: AttentionRow[] = [];
	for (const health of services) {
		const row: ServiceRow = { health };
		const state = stateLabel(row, now);
		if (!needsAttention(row, now)) continue;
		out.push({
			id: health.id,
			name: health.name,
			tone: state.tone,
			icon: state.icon,
			state: state.word,
			detail: state.detail,
			problem: health.problem?.trim() ?? ''
		});
	}
	return out;
}

// Re-exported through this module so the screen imports one vocabulary rather
// than reaching into the Services screen's file for half of it.
export { needsAttention };

/**
 * What the page head says, and it is derived from the same data the blocks
 * are.
 *
 * A CONSTANT HERE IS THE FAILURE THIS FUNCTION EXISTS TO PREVENT, and the
 * mockup records it happening: a fixed "Last delta sync 14:02, 6 minutes ago"
 * sat above "No services configured" on the very first screen a new user saw —
 * UsArr reporting a completed sync over a system with nothing connected, first
 * in reading order and first in the accessibility tree.
 *
 * `count` is the roll-up's count string (`1 error, 2 warnings`) or ''. It is
 * passed in rather than recomputed so the head and the block cannot disagree.
 */
export function headline(mode: HomeMode | undefined, services: number, count: string): string {
	if (mode === undefined) return 'Reading what is connected.';
	if (mode === 'unconfigured') return 'No service is connected yet.';
	const connected = `${services} ${services === 1 ? 'service' : 'services'} connected.`;
	// "Nothing needs attention" is a claim about what was computed, and it is
	// exactly what an empty roll-up means. "Every service is healthy" is a
	// stronger claim than the roll-up makes — it skips an enabled instance
	// whose probe ran and could not classify it — so it is not the sentence.
	const state = count === '' ? 'Nothing needs attention.' : `${count}.`;
	if (mode === 'search-and-grab') return `Search-and-Grab mode. ${connected} ${state}`;
	return `${connected} ${state}`;
}
