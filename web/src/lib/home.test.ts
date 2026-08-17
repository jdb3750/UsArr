import { describe, expect, it } from 'vitest';
// HOME'S TEMPLATE AS TEXT, for the copy guard at the bottom of this file. `?raw`
// is Vite's own mechanism and it types as `string`, which is what lets a ban
// list run over inline markup in an `environment: 'node'` vitest run with no
// Svelte plugin. See `$lib/copyguard` for why the stripping is not written here.
import HOME_SOURCE from '../routes/+page.svelte?raw';
import { sectionsMarkup, userFacingMarkup } from './copyguard';
import {
	attention,
	hasIndexer,
	headline,
	homeMode,
	HOME_SEARCH_SCOPE_NOTE,
	needsAttention
} from './home';
// §17.5's ban list, imported rather than restated: Home renders the same grab
// vocabulary the Requests block does, and two copies of a ban list is two lists.
import { FORBIDDEN_OUTCOME_WORDS } from './requests';
import { rollup, rollupCount, type ServiceRow } from './services';
import type { ServiceHealth, ServicesHealth } from './api';

/**
 * HOME's vocabulary, pinned. ARCHITECTURE.md §17.2 (as amended by ADR-0028),
 * §17.7 and §8.5.
 *
 * Every assertion here is a rule one of those states as a rule rather than as
 * taste, and the ones that are easiest to break silently are:
 *
 *  1. Search-and-Grab mode is DERIVED from the health response's `role`, which
 *     is §8.5's own test — "no configured instance advertises LibrarySync" —
 *     and never from a constant. A hard-coded mode goes on claiming there is
 *     no library on the first build that connects a Sonarr.
 *  2. Nothing configured is `unconfigured` and never an empty Home (§17.7).
 *  3. Block B is hidden when empty, so `attention()` must return nothing at
 *     all rather than a row that says everything is fine. A green "all good"
 *     panel is the thing the block must never become.
 *  4. Block B and the Services roll-up answer the same question and must agree
 *     on the count, which is why both go through `needsAttention`.
 *  5. The page head is derived too. The mockup records the failure: a constant
 *     "Last delta sync 14:02" sat above "No services configured" on the first
 *     screen a new user saw.
 */

const NOW = new Date('2026-08-16T14:08:00Z');

function health(over: Partial<ServiceHealth> = {}): ServiceHealth {
	return {
		id: 1,
		name: 'Prowlarr',
		kind: 'prowlarr',
		role: 'indexer',
		baseUrl: 'http://10.0.0.4:9696',
		enabled: true,
		state: 'healthy',
		breakerState: 'closed',
		consecutiveFailures: 0,
		warnings: [],
		blockedIndexers: [],
		stale: false,
		...over
	};
}

function payload(over: Partial<ServicesHealth> = {}): ServicesHealth {
	return { services: [], anyUnhealthy: false, setupRequired: false, ...over };
}

describe('homeMode', () => {
	it('is unconfigured when the server says setup is required', () => {
		expect(homeMode(payload({ setupRequired: true }))).toBe('unconfigured');
	});

	it('is unconfigured when nothing came back, even without the flag', () => {
		// A response with no services and no flag is still nothing configured, and
		// §17.7 sends that to the first-run path rather than to an empty Home.
		expect(homeMode(payload())).toBe('unconfigured');
	});

	it('is Search-and-Grab when every configured instance is an indexer', () => {
		expect(homeMode(payload({ services: [health()] }))).toBe('search-and-grab');
	});

	it('leaves Search-and-Grab as soon as one instance is library-bearing', () => {
		// §8.5's activation test is "no configured instance advertises
		// LibrarySync", so ONE library-bearing service is enough to leave the
		// mode. The role is what carries it; nothing here is hard-coded to
		// prowlarr.
		const rows = [health(), health({ id: 2, name: 'Radarr', kind: 'radarr', role: 'library' })];
		expect(homeMode(payload({ services: rows }))).toBe('library');
	});

	it('reads the role rather than the kind, so an unknown kind still classifies', () => {
		const rows = [health({ kind: 'komga', role: 'library' })];
		expect(homeMode(payload({ services: rows }))).toBe('library');
	});
});

describe('attention', () => {
	it('is empty when every service is healthy, so Block B is hidden', () => {
		// §17.2: hidden ENTIRELY when empty, never a green "all good" panel. The
		// block's {#if} keys on this being empty, so a row describing health here
		// would put the panel on the screen.
		expect(attention([health(), health({ id: 2, name: 'Prowlarr 4K' })], NOW)).toEqual([]);
	});

	it('carries UsArr’s own word and the upstream’s verbatim text separately', () => {
		const rows = attention(
			[health({ state: 'down', consecutiveFailures: 3, problem: '401 Unauthorized\nbody: {}' })],
			NOW
		);
		expect(rows).toHaveLength(1);
		// State is UsArr speaking. The mechanism's vocabulary — `down`, `breaker
		// open` — belongs behind the Services row expander and not here.
		expect(rows[0].state).toBe('not answering');
		expect(rows[0].detail).toBe('3 failed attempts');
		// Problem is the upstream's own words, whole and untouched. The screen
		// truncates for the cell; the row carries the full string.
		expect(rows[0].problem).toBe('401 Unauthorized\nbody: {}');
		expect(rows[0].tone).toBe('err');
	});

	it('reports a service that has never been probed rather than assuming it is fine', () => {
		const rows = attention([health({ stale: true })], NOW);
		expect(rows).toHaveLength(1);
		expect(rows[0].state).toBe('not checked yet');
		// `none`, not `ok`: nothing was observed, and a green tick would claim a
		// measurement UsArr has not taken.
		expect(rows[0].tone).toBe('none');
	});

	it('reports a service the owner turned off', () => {
		const rows = attention([health({ enabled: false })], NOW);
		expect(rows.map((r) => r.state)).toEqual(['turned off']);
	});

	it('has no problem text when the upstream said nothing', () => {
		expect(attention([health({ enabled: false })], NOW)[0].problem).toBe('');
	});
});

describe('Block B and the Services roll-up agree', () => {
	// Two lists answering "what is wrong" from two predicates is how one screen
	// reports three problems while the other reports four, and the only reading
	// available to the user is that one of them is lying. Both go through
	// `needsAttention`, and this is the test that keeps them there.
	const services = [
		health(),
		health({ id: 2, name: 'Down', state: 'down', consecutiveFailures: 2 }),
		health({ id: 3, name: 'Off', enabled: false }),
		health({ id: 4, name: 'Unprobed', stale: true }),
		health({ id: 5, name: 'Wobbly', state: 'degraded', consecutiveFailures: 1 })
	];
	const rows: ServiceRow[] = services.map((h) => ({ health: h }));

	it('selects the same instances', () => {
		expect(attention(services, NOW).map((r) => r.id)).toEqual(rollup(rows, NOW).map((r) => r.id));
	});

	it('counts them the same way', () => {
		expect(rollupCount(attention(services, NOW))).toBe(rollupCount(rollup(rows, NOW)));
	});

	it('keeps a healthy row out of both', () => {
		expect(needsAttention({ health: health() }, NOW)).toBe(false);
	});
});

describe('headline', () => {
	it('says it is still reading before the response lands', () => {
		expect(headline(undefined, 0, '')).toBe('Reading what is connected.');
	});

	it('never reports a sync over a system with nothing connected', () => {
		// The failure the mockup records, as a test: a constant "Last delta sync
		// 14:02, 6 minutes ago" above "No services configured".
		const line = headline('unconfigured', 0, '');
		expect(line).toBe('No service is connected yet.');
		expect(line).not.toMatch(/sync/i);
	});

	it('names the mode and the count', () => {
		expect(headline('search-and-grab', 2, '1 error, 1 warning')).toBe(
			'Search-and-Grab mode. 2 services connected. 1 error, 1 warning.'
		);
	});

	it('singularises one service', () => {
		expect(headline('search-and-grab', 1, '')).toBe(
			'Search-and-Grab mode. 1 service connected. Nothing needs attention.'
		);
	});

	it('claims nothing needs attention rather than that every service is healthy', () => {
		// The roll-up skips an enabled instance whose probe ran and could not
		// classify it, so an empty roll-up does not establish that everything is
		// healthy. It establishes exactly what the shorter claim says.
		const line = headline('search-and-grab', 3, '');
		expect(line).toContain('Nothing needs attention.');
		expect(line).not.toMatch(/healthy/i);
	});

	it('does not claim Search-and-Grab once a library-bearing service exists', () => {
		expect(headline('library', 1, '')).not.toMatch(/Search-and-Grab/);
	});
});

/**
 * THE SEARCH ENTRY POINT'S TWO RULES, and both are rules rather than taste.
 *
 *  1. A box is drawn only where something can answer it. `hasIndexer` is the
 *     precondition, NOT the mode: an install with a Sonarr and a Prowlarr is in
 *     `library` mode and has a perfectly good indexer, and gating on the mode
 *     would delete a working control the day a Sonarr is accepted.
 *  2. The box says what it searches. DESIGN-DIRECTION §8.3 keeps library search
 *     and release search apart — merging them is how a 0 ms local query waits on
 *     a 30 s indexer — so an unlabelled input, which reads as library search by
 *     default, is the failure.
 */
describe('hasIndexer', () => {
	it('is false on a fresh install, so Home never draws a box with nothing behind it', () => {
		expect(hasIndexer(payload())).toBe(false);
	});

	it('is true whenever an indexer is configured', () => {
		expect(hasIndexer(payload({ services: [health()] }))).toBe(true);
	});

	it('stays true in library mode, because an indexer beside a Sonarr still answers', () => {
		const rows = [health(), health({ id: 2, name: 'Radarr', kind: 'radarr', role: 'library' })];
		const health_ = payload({ services: rows });
		expect(homeMode(health_)).toBe('library');
		expect(hasIndexer(health_)).toBe(true);
	});

	it('is false when the only service is library-bearing', () => {
		const rows = [health({ kind: 'radarr', role: 'library' })];
		expect(hasIndexer(payload({ services: rows }))).toBe(false);
	});
});

describe('HOME_SEARCH_SCOPE_NOTE', () => {
	it('names what is searched and where the results are', () => {
		expect(HOME_SEARCH_SCOPE_NOTE).toMatch(/indexers/i);
		expect(HOME_SEARCH_SCOPE_NOTE).toMatch(/Requests/);
	});

	it('says explicitly that it is not your own library', () => {
		// The whole point of the string. A box that only says "Search" is the
		// merge §8.3 forbids, arrived at by omission.
		expect(HOME_SEARCH_SCOPE_NOTE).toMatch(/not your own library/i);
	});
});

/* ── §17.5's ban, over Home's own Recent-grabs chrome ─────────────────────── */

/**
 * THE HOLE THIS CLOSES, STATED FIRST, BECAUSE IT WAS REAL AND NOT HYPOTHETICAL.
 *
 * §17.5's banned vocabulary is checked against shipped strings in three places
 * and, until this block, all three were on the Requests side of the app:
 * `requests.test.ts` holds the strings `requests.ts` exports, the markup of
 * `routes/requests/+page.svelte`, and the markup of `$lib/RecentGrabs.svelte`.
 *
 * Home draws grab copy too, and NONE of it was any of those three. The rows
 * themselves are the shared component and were covered the day it was extracted
 * — but the CHROME around them is Home's own: the `Recent grabs` heading, the
 * count beside it (`3 grabs`, or `the 10 most recent`), the show-all link, and
 * the whole unreadable-list banner, which is four sentences about what a
 * missing list does and does not mean. Every one of those is a statement about
 * a grab, written inline in `routes/+page.svelte`, and no guard read the file.
 * A `failed` typed into that banner would have shipped green.
 *
 * ⚠️ THE REGION IS BOTH ARMS OF THE `{#if}`, WHICH IS WHY THE SLICER RETURNS A
 * LIST. Home renders `id="home-grabs"` twice — once for the error banner and
 * once for the loaded table — and a guard that took the first match would have
 * read the banner and silently dropped the heading, the count and the link.
 * `sectionsMarkup` throws rather than returning nothing when the marker moves,
 * for the reason the Requests guard records: an empty corpus passes every
 * `not.toContain` there is.
 *
 * WHAT THIS BLOCK DELIBERATELY DOES NOT DO. It does not re-check the rows: they
 * are `$lib/RecentGrabs.svelte`, which `requests.test.ts` already holds against
 * the same list, and a second copy of that assertion would be the duplication
 * the component was extracted to remove. And it does not extend the subset ban
 * across the REST of Home — the search entry point and the three mode blocks —
 * which is a real gap and a separate change, because those are statements about
 * a search and about configuration rather than about a download.
 */
const HOME_GRAB_MARKUP = sectionsMarkup(userFacingMarkup(HOME_SOURCE), 'id="home-grabs"').join(
	'\n'
);

describe('the banned vocabulary, in Home’s Recent-grabs chrome', () => {
	it('is reading the region it thinks it is reading', () => {
		// Both arms, each pinned to a string only that arm carries. A guard that
		// matches nothing is indistinguishable from no guard, and after the
		// `{#if}` there are two ways to match half of it rather than one.
		expect(HOME_GRAB_MARKUP).toContain('Recent grabs');
		expect(HOME_GRAB_MARKUP).toContain('Your recent grabs could not be read');
		expect(HOME_GRAB_MARKUP).toContain('most recent');
		expect(HOME_GRAB_MARKUP).toContain('All recent grabs on Requests');
	});

	it.each(FORBIDDEN_OUTCOME_WORDS)('never says “%s” about a grab', (word) => {
		expect(HOME_GRAB_MARKUP.toLowerCase()).not.toContain(word);
	});

	it('never asserts in the markup that a grab did not happen', () => {
		// The same two phrases the Requests guard pins, and the reason Home needs
		// them is that its banner is the one place in the app that comes closest:
		// "Nothing is missing here because a grab did not happen" is the DENIAL of
		// the claim, and an edit that dropped its first four words would turn it
		// into the assertion §17.5 forbids.
		expect(HOME_GRAB_MARKUP.toLowerCase()).not.toContain('did not go through');
		expect(HOME_GRAB_MARKUP.toLowerCase()).not.toContain('nothing was sent');
	});

	it('keeps saying that an unreadable list is not an empty one', () => {
		// The counter-case, pinned so a later tightening of the ban has to face
		// it. This sentence is the point of the banner: a local read failing says
		// something about UsArr, not about whether anything was ever grabbed.
		expect(HOME_GRAB_MARKUP).toContain('a grab did not happen');
	});
});
