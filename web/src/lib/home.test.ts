import { describe, expect, it } from 'vitest';
import { attention, headline, homeMode, needsAttention } from './home';
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
