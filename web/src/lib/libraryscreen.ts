/**
 * THE `/library` SCREEN'S MODEL — the whole-catalogue "Recently added" list.
 *
 * WHAT THE SCREEN IS, AND WHAT IT DELIBERATELY IS NOT. ARCHITECTURE.md §16 also
 * names a per-type library GRID with a sort control and a library scope. ⚠️ THIS
 * HEADER USED TO SAY THAT NONE OF IT COULD BE BUILT BECAUSE THERE WAS NO
 * `GET /api/v1/library`. There is one now, and the grid is a SEPARATE screen
 * over it — `routes/library/[type]`, modelled by `$lib/librarygrid`. This screen
 * did not become its predecessor: §17.2 keeps both, and http-api.md §7 is
 * explicit that the browse read is *"a different endpoint from §1, not a
 * superset of it"*, because Block C is closed at one table, one order and no
 * filters. `GET /api/v1/library/recent` still parses only `limit` and `cursor`
 * and is still hard-ordered `added_at DESC, id DESC`, which is what makes a
 * filter or a sort ON THIS SCREEN a control over a keyset prefix. Covers are
 * absent from both: no endpoint in the mux serves an image.
 *
 * WHY THE COPY LIVES HERE RATHER THAN IN THE TEMPLATE. `vitest.config.ts` is
 * `environment: 'node'` with no Svelte plugin, so a rule inside an `{#if}` in a
 * `+page.svelte` is a rule nothing can test — the same reason `$lib/home`,
 * `$lib/search` and `$lib/library` hold their screens' decisions. The decision
 * held here is which empty state is TRUE, which is the one thing on this screen
 * that can silently lie: an empty catalogue means three different things
 * depending on what is connected, and rendering the wrong one tells a user with
 * no library-bearing service that an import is on its way.
 *
 * IT IS NOT A SECOND COPY OF THE RECENT FEED. The paging, the stop rule, the
 * media-type vocabulary and the availability rendering all belong to
 * `$lib/library` and are imported from there by the route.
 *
 * ⚠️ AND `recentEmptyState` IS SHARED WITH THE PER-TYPE GRID, deliberately.
 * `$lib/librarygrid.browseEmptyState` answers the three non-`library` modes by
 * calling it, because those three are facts about what is CONNECTED rather than
 * about a media type, and two screens telling one install two stories about its
 * own services is the drift this module was written to prevent.
 */

import type { HomeMode } from './home';

/** One empty state: §9.6's one-sentence heading plus the explanation under it. */
export interface RecentEmptyState {
	title: string;
	text: string;
}

/**
 * ⚠️ HOME'S BLOCK C WORDING, VERBATIM, AND THE MATCH IS GUARDED.
 *
 * Both screens draw the same table over the same endpoint, so an install with
 * a connected library and no rows yet must not be told two different stories
 * depending on which screen it is read from. `libraryscreen.test.ts` reads
 * `routes/+page.svelte` as text and fails if these two strings stop matching
 * the ones Block C passes to `<List>`, which turns a silent divergence into a
 * failing test that names both files.
 */
const LIBRARY_EMPTY: RecentEmptyState = {
	title: 'Nothing catalogued yet',
	text:
		'A library-bearing service is connected and UsArr has not written any catalogue rows from it ' +
		'yet. The first full import starts on its own the first time UsArr builds a connection to a ' +
		'service that has never completed one, and on a large library it runs for minutes rather than ' +
		'seconds. This table fills in as those rows are written.'
};

/**
 * WHY AN EMPTY LIST IS EMPTY, ANSWERED FROM THE SERVICES READ AND NOT GUESSED.
 *
 * `undefined` is the fourth case and it is not "loading": the route only asks
 * for an empty state once the services read has settled, so `undefined` means
 * that read FAILED. Saying "nothing is catalogued yet" on the strength of a
 * failed health read would assert the library is empty when what is actually
 * known is that UsArr could not find out.
 *
 * ⚠️ NONE OF THESE FOUR IS A DEGRADED STATE FOR THE CATALOGUE, and the route
 * enforces the other half: a service in a bad state greys nothing out here.
 * ARCHITECTURE.md §17.7's own rule for a degraded instance is that **the
 * catalogue does not grey out**: browse and search keep working from the
 * replica, because every row on this screen is a local SQLite row that is
 * exactly as true as it was before the service broke.
 */
export function recentEmptyState(mode: HomeMode | undefined): RecentEmptyState {
	if (mode === 'library') return LIBRARY_EMPTY;
	if (mode === 'search-and-grab') {
		return {
			title: 'No library-bearing service is connected',
			text:
				'Every service configured here is an indexer, and an indexer has no catalogue for UsArr ' +
				'to replicate, so there is nothing for this screen to list. Kavita is the library-bearing ' +
				'service this build connects, and UsArr imports its catalogue once, on that first connect.'
		};
	}
	if (mode === 'unconfigured') {
		return {
			title: 'No services configured',
			text:
				'UsArr talks to the services you already run, and none of them is connected yet. Connect ' +
				'one on the Services screen and whatever it catalogues is listed here.'
		};
	}
	return {
		title: 'Nothing to list, and no reason to give',
		text:
			'UsArr holds no catalogue rows, and the read that says which services are connected did not ' +
			'answer, so this screen cannot tell you whether nothing is connected or an import has not ' +
			'run yet. The Services screen is where that question is answered.'
	};
}
