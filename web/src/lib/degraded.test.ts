/*
 * §17.7'S DEGRADED-BACKEND BANNER, GUARDED IN BOTH DIRECTIONS.
 *
 * ⚠️ THE SECOND DIRECTION IS THE ONE THAT MATTERS, and it is why this file is
 * shaped the way it is. A guard that only ever sees a degraded instance cannot
 * tell "correct" from "always fires": `() => [notice]` passes every assertion
 * about what the banner says, and would put a permanent "is not answering"
 * across the catalogue of a perfectly healthy install. So every fixture here
 * carries a healthy instance beside the broken one, and the healthy one is
 * asserted to produce NOTHING.
 *
 * ⚠️ AND NON-VACUITY IS ASSERTED BEFORE ANYTHING ELSE. The failure mode of a
 * guard over a collection is the empty collection: `degradedNotices()` over no
 * services returns no notices, which satisfies every `not.toContain` and every
 * length check written the lazy way. `has something to be wrong about` pins the
 * fixture's size AND that it holds one of each kind, so a fixture that lost its
 * rows fails there rather than passing everywhere.
 *
 * WHAT CAN GO WRONG, in the order it would go wrong silently:
 *
 *   1. The banner fires on a healthy instance. See above.
 *   2. It stops firing on a degraded one — a filter narrowed, a state string
 *      renamed on the wire — and the catalogue silently starts presenting old
 *      rows as current, which is the exact dishonesty §17.7 exists to prevent.
 *   3. It renders a timestamp off `null`. `lastFullSyncAt` is `null` for "no
 *      full import has ever completed", which is not a time; softening it into
 *      one is the reassuring-and-wrong number §17.7 calls worse than no banner.
 *   4. The number loses its relative half. DESIGN-DIRECTION §9.1 names THIS
 *      banner as its worked example: a bare `HH:MM` reads identically whether
 *      the instance has been down for six minutes or twenty-two hours.
 *   5. The template stops rendering one of §17.7's required parts — the
 *      user-given name, the state word, the timestamp, the link to Services —
 *      or starts greying the catalogue out.
 *
 * WHY IT IS SPLIT IN TWO. `vitest.config.ts` is `environment: 'node'` with no
 * Svelte plugin, so a component cannot be compiled, imported or rendered here.
 * Defects 1-4 live in `degradedNotices()`, a plain function this file calls for
 * real. Defect 5 lives in markup and can only be read as text, which is
 * `services-screen.test.ts`'s idiom.
 */

import { describe, expect, it } from 'vitest';
import { degradedNotices } from './degraded';
import { stampOf } from './services';
import type { ServiceHealth, ServicesHealth } from './api';
import { userFacingMarkup } from './copyguard';
// The banner AS TEXT. Its markup is the one thing this suite cannot execute.
import BANNER_SOURCE from './DegradedBanner.svelte?raw';
// Both catalogue screens as text, to pin that they still MOUNT it. A perfect
// component nothing renders is defect 2 arriving through the other door.
import LIBRARY_SOURCE from '../routes/library/+page.svelte?raw';
import TYPE_SOURCE from '../routes/library/[type]/+page.svelte?raw';

/** A fixed instant, so the relative half of every stamp is a constant. */
const NOW = new Date('2026-08-21T14:08:00Z');

function health(over: Partial<ServiceHealth> = {}): ServiceHealth {
	return {
		id: 1,
		name: 'Kavita',
		kind: 'kavita',
		role: 'library',
		baseUrl: 'http://10.0.0.7:5000',
		enabled: true,
		state: 'healthy',
		breakerState: 'closed',
		consecutiveFailures: 0,
		warnings: [],
		blockedIndexers: [],
		// The default is FALSE and it is never varied in this file on purpose:
		// `stale` means "no probe has been taken yet" and has nothing to do with
		// data age. If a future edit starts reading it for freshness, no assertion
		// here will move — which is why the markup half asserts it by name.
		stale: false,
		lastFullSyncAt: '2026-08-21T14:02:00Z',
		workCount: 1240,
		fileReadFailures: 0,
		...over
	};
}

/**
 * THE FIXTURE, AND EVERY ROW IN IT IS A STATE A REAL INSTALL REACHES.
 *
 * Two catalogue instances of the same kind, deliberately: §17.7's whole reason
 * for naming the user's name rather than the application is a stack running two
 * of the same thing, and a fixture with one Kavita cannot tell the two apart.
 */
const SERVICES: ServiceHealth[] = [
	// DEGRADED, with a sync time. The funded branch (§16.1), and the only shape
	// that produces a banner.
	health({ id: 1, name: 'Kavita (comics)', state: 'down', consecutiveFailures: 4 }),
	// HEALTHY, and the twin of the row above. Its silence is the check.
	health({ id: 2, name: 'Kavita (manga)', state: 'healthy' }),
	// DEGRADED AND NEVER IMPORTED. `null` is not a time and is not softened into
	// one; `$lib/degraded` argues why this state is silent here.
	health({
		id: 3,
		name: 'Komga',
		kind: 'komga',
		state: 'down',
		lastFullSyncAt: null,
		workCount: 0
	}),
	// DEGRADED AFTER A PARTIAL IMPORT: `null` WITH rows behind it, which is a
	// different fact from the row above and reaches the same silence.
	health({
		id: 4,
		name: 'Audiobookshelf',
		kind: 'audiobookshelf',
		state: 'degraded',
		lastFullSyncAt: null,
		workCount: 812
	}),
	// A DEGRADED INDEXER. It contributes no catalogue, so "showing cached data
	// from" would date rows it never wrote.
	health({
		id: 5,
		name: 'Prowlarr',
		kind: 'prowlarr',
		role: 'indexer',
		state: 'down',
		lastFullSyncAt: null,
		workCount: 0
	})
];

const ALL: ServicesHealth = { services: SERVICES, anyUnhealthy: true, setupRequired: false };

/** Only the healthy twin, which is what an install with nothing wrong looks like. */
const HEALTHY_ONLY: ServicesHealth = {
	services: SERVICES.filter((s) => s.state === 'healthy'),
	anyUnhealthy: false,
	setupRequired: false
};

describe('degradedNotices', () => {
	it('has something to be wrong about', () => {
		// The floor under every assertion below. An emptied fixture would satisfy
		// all of them — "no banner on a healthy instance" is vacuously true over no
		// instances at all — so the shape is pinned before it is used.
		expect(SERVICES.length, 'the fixture is empty, so nothing below is a check').toBe(5);
		expect(
			SERVICES.filter((s) => s.state !== 'healthy').length,
			'no fixture instance is degraded, so the positive direction never fires'
		).toBe(4);
		expect(
			SERVICES.filter((s) => s.state === 'healthy').length,
			'no fixture instance is healthy, so the negative direction is untested'
		).toBe(1);
		expect(
			HEALTHY_ONLY.services.length,
			'the healthy-only fixture is empty, so its silence proves nothing'
		).toBeGreaterThan(0);
	});

	it('fires on a degraded instance that has a sync time', () => {
		const notices = degradedNotices(ALL, NOW);
		expect(notices.map((n) => n.name)).toEqual(['Kavita (comics)']);
		const [only] = notices;
		// §17.3's vocabulary, not the wire's: `down` never reaches a user.
		expect(only.word).toBe('not answering');
		expect(only.word).not.toContain('down');
		// That instance's OWN timestamp, absolute AND relative (§9.1). Compared
		// against `stampOf` itself so this pins the FIELD and the FORMATTER rather
		// than re-implementing en-GB date rules in a second place.
		expect(only.stamp).toBe(stampOf('2026-08-21T14:02:00Z', NOW));
		expect(only.stamp).toContain('14:02');
		expect(only.stamp).toContain('6 minutes ago');
	});

	it('stays silent on a healthy instance, which is the half that proves the rest', () => {
		expect(degradedNotices(HEALTHY_ONLY, NOW)).toEqual([]);
		// And in the mixed fixture the healthy twin is absent by NAME, so a filter
		// that widened to every instance fails here rather than in a length check
		// that a fixture change could quietly satisfy.
		expect(degradedNotices(ALL, NOW).map((n) => n.name)).not.toContain('Kavita (manga)');
	});

	it('never renders a banner off a null timestamp, with rows or without', () => {
		const named = degradedNotices(ALL, NOW).map((n) => n.name);
		expect(named).not.toContain('Komga');
		expect(named).not.toContain('Audiobookshelf');
		// Not silence by accident: both ARE degraded, and the partial-import one has
		// real replica rows behind it. Neither may acquire a time it does not have.
		expect(SERVICES.filter((s) => s.lastFullSyncAt === null && s.state !== 'healthy').length).toBe(
			3
		);
		expect(degradedNotices(ALL, NOW).every((n) => n.stamp !== '')).toBe(true);
	});

	it('leaves a degraded indexer alone, which has no cached catalogue to date', () => {
		expect(degradedNotices(ALL, NOW).map((n) => n.name)).not.toContain('Prowlarr');
	});
});

describe('the banner template renders what §17.7 requires', () => {
	const markup = userFacingMarkup(BANNER_SOURCE);

	it('has a corpus to read', () => {
		expect(
			markup.length,
			'the banner markup came back empty, so nothing below is a check'
		).toBeGreaterThan(200);
		expect(markup).toContain('class="banner');
	});

	it('draws the name, the state word, the timestamp and the link', () => {
		expect(markup).toContain('{notice.name}');
		expect(markup).toContain('{notice.word}');
		expect(markup).toContain('{notice.stamp}');
		// §17.7's link to the screen that owns the fix, in `.banner__actions` —
		// the slot List.svelte never emits, which is half of why this is a sibling.
		expect(markup).toContain('banner__actions');
		expect(markup).toContain('href={servicesPath}');
	});

	it('never renders the kind instead of the user-given name', () => {
		// §17.7: naming the application rather than the instance is what stops a
		// stack running two of the same thing from being legible.
		expect(markup).not.toContain('notice.kind');
		expect(markup).not.toContain('applicationName');
	});

	it('adds no motion and greys nothing out', () => {
		for (const banned of ['transition:', 'animate:', 'animation', 'opacity', 'skeleton']) {
			expect(
				markup,
				`${banned} in a banner that must not animate or grey the catalogue`
			).not.toContain(banned);
		}
	});

	it('reads no freshness out of `stale`, which does not mean that', () => {
		// The third sense of `stale` this tree does not need: on the wire it is
		// "no probe has been taken yet", on ListState it is "real but old".
		expect(BANNER_SOURCE).not.toContain('notice.stale');
		expect(BANNER_SOURCE).not.toContain('health.stale');
	});
});

describe('both catalogue screens mount it', () => {
	for (const [name, source] of [
		['routes/library/+page.svelte', LIBRARY_SOURCE],
		['routes/library/[type]/+page.svelte', TYPE_SOURCE]
	] as const) {
		it(`${name} renders <DegradedBanner> above its table`, () => {
			const markup = userFacingMarkup(source);
			expect(markup.length, `${name} came back empty`).toBeGreaterThan(1000);
			expect(markup, `${name} stopped mounting the banner`).toContain('<DegradedBanner');
			// ABOVE the list, not inside it: §17.7 wants the table untouched, and a
			// banner passed into <List> would be the staleNote route this deliberately
			// does not take.
			const banner = markup.indexOf('<DegradedBanner');
			const list = markup.indexOf('<List');
			expect(list, `${name} stopped rendering a <List>`).toBeGreaterThan(-1);
			expect(banner, `${name} moved the banner below its table`).toBeLessThan(list);
		});
	}
});
