import { describe, expect, it } from 'vitest';
import {
	apiPathOf,
	applicationName,
	classify403,
	clockOf,
	connectedHeadline,
	dateOf,
	describeSync,
	firstLine,
	itemsCell,
	likelyCauses,
	mechanics,
	normaliseUrlBase,
	problemCell,
	relativeTo,
	rollup,
	rollupCount,
	stampOf,
	stateLabel,
	testOutcome,
	testTitle,
	testTone,
	syncCell,
	type ServiceRow
} from './services';
import type { ServiceHealth } from './api';

/**
 * The Services screen's vocabulary, pinned.
 *
 * Every assertion here is a rule ARCHITECTURE.md §17.3 states as a rule rather
 * than as taste, and each one had already been got wrong once by the time it
 * was written down:
 *
 *  1. `State` is UsArr's own words. `degraded / breaker open` and `needs
 *     re-identification` are the mechanism's names and they belong behind the
 *     expander, which is what `mechanics()` is asserted to still carry.
 *  2. `Problem` is the upstream's own words and NOTHING else. No cell may hold
 *     a rendering decision or a sentence beginning "Nothing is wrong."
 *  3. The three 403s are told apart on the body's `error` field and never on
 *     the status, because only `csrf` is retryable and an unknown code must
 *     default to "surface it".
 *  4. `Not applicable` and `—` are different words for different facts, and an
 *     indexer's item count is the first, never the second and never `0`.
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

function row(over: Partial<ServiceHealth> = {}): ServiceRow {
	return { health: health(over) };
}

describe('the State column speaks UsArr, not the mechanism', () => {
	it('renders a tripped breaker as `paused` with the retry time, never `breaker open`', () => {
		const label = stateLabel(
			row({
				state: 'down',
				breakerState: 'open',
				consecutiveFailures: 7,
				breakerRetryAt: '2026-08-16T14:19:00Z'
			}),
			NOW
		);
		expect(label.word).toBe('paused');
		expect(label.detail).toBe('7 failed attempts, retrying 14:19, in 11 minutes');
		expect(label.tone).toBe('err');
		expect(`${label.word} ${label.detail}`).not.toMatch(/breaker|open|degraded/i);
	});

	it('names the application on a re-identification, never the state string', () => {
		const label = stateLabel(row({ kind: 'sonarr', state: 'needs re-identification' }), NOW);
		expect(label.word).toBe('this may be a different Sonarr');
		expect(label.word).not.toContain('re-identification');
	});

	it('says `not answering` for a down instance whose breaker has not tripped', () => {
		const label = stateLabel(row({ state: 'down', consecutiveFailures: 1 }), NOW);
		expect(label.word).toBe('not answering');
		expect(label.detail).toBe('1 failed attempt');
	});

	it('says `some attempts are failing` rather than `degraded`', () => {
		const label = stateLabel(row({ state: 'degraded', consecutiveFailures: 2 }), NOW);
		expect(label.word).toBe('some attempts are failing');
		expect(label.word).not.toContain('degraded');
		expect(label.tone).toBe('warn');
	});

	it('distinguishes a disabled service from an unprobed one', () => {
		expect(stateLabel(row({ enabled: false, state: 'unknown' }), NOW).word).toBe('turned off');
		const fresh = stateLabel(row({ state: 'unknown', stale: true }), NOW);
		expect(fresh.word).toBe('not checked yet');
		expect(fresh.detail).toBe('no probe has run');
	});

	it('leaves a healthy row with nothing to qualify', () => {
		const label = stateLabel(row(), NOW);
		expect(label).toMatchObject({ word: 'healthy', tone: 'ok', detail: '' });
	});

	it('refuses to call a never-probed instance healthy, whatever the API state says', () => {
		// handleServicesHealth answers `healthy` on a stored last_ok_at alone, so
		// this row is the one a service that answered once when it was added and
		// has not been contacted since produces. A green tick here claims a
		// measurement that was never taken.
		const label = stateLabel(row({ state: 'healthy', stale: true }), NOW);
		expect(label.word).toBe('not checked yet');
		expect(label.tone).not.toBe('ok');
		expect(label.detail).toBe('no probe has run');
	});
});

describe('the mechanism survives, behind the expander', () => {
	it('keeps the breaker vocabulary the State column refuses', () => {
		const parts = mechanics(
			row({
				breakerState: 'open',
				consecutiveFailures: 7,
				breakerRetryAt: '2026-08-16T14:19:00Z',
				observedAt: '2026-08-16T14:07:00Z'
			}),
			NOW
		);
		expect(parts[0]).toBe('State OPEN');
		expect(parts[1]).toBe('Next probe 14:19, in 11 minutes');
		expect(parts[2]).toBe('Consecutive failures 7');
		expect(parts[3]).toBe('Last probed 14:07, 1 minute ago');
	});

	it('says so when nothing has ever been probed', () => {
		expect(mechanics(row({ stale: true }), NOW)).toContain('Never probed');
	});
});

describe('the Problem column holds one object and nothing else', () => {
	it('is the em-dash nothing-word when the upstream said nothing', () => {
		expect(problemCell(row())).toMatchObject({ text: '—', muted: true, sub: '' });
	});

	it('summarises with the upstream first line, never with a paraphrase', () => {
		const verbatim =
			'GET /api/v1/indexer 401 Unauthorized\n{"message":"Unauthorized","description":"needs X-Api-Key"}';
		const cell = problemCell(row({ problem: verbatim }));
		expect(cell.text).toBe('GET /api/v1/indexer 401 Unauthorized');
		expect(verbatim.startsWith(cell.text)).toBe(true);
		expect(cell.muted).toBe(false);
	});

	it('keeps a single-line body whole', () => {
		expect(firstLine('dial tcp 10.0.0.4:9696: connect: connection refused')).toBe(
			'dial tcp 10.0.0.4:9696: connect: connection refused'
		);
	});
});

describe('Items and Last successful sync are `Not applicable`, not zero', () => {
	it('says an indexer contributes no catalogue rather than showing 0', () => {
		expect(itemsCell(row())).toMatchObject({ text: 'Not applicable', muted: true });
		expect(syncCell(row())).toMatchObject({ text: 'Not applicable', sub: 'no catalogue' });
	});

	it('keeps `Never` for a catalogue source that has not synced', () => {
		expect(syncCell(row({ kind: 'sonarr', role: 'catalogue' })).text).toBe('Never');
	});
});

describe('the sync-channel vocabulary §17.3 fixes now so it cannot be retrofitted', () => {
	it('labels an ordered delta with its channel', () => {
		expect(describeSync({ kind: 'delta', at: '2026-08-16T14:02:00Z', ordered: true }, NOW)).toEqual(
			{
				text: 'delta 14:02',
				sub: '6 minutes ago',
				muted: false
			}
		);
	});

	it('labels a channel-3b source `page-walk delta`', () => {
		const cell = describeSync(
			{ kind: 'page-walk delta', at: '2026-08-16T13:40:00Z', ordered: true },
			NOW
		);
		expect(cell.text).toBe('page-walk delta 13:40');
		expect(cell.sub).toBe('28 minutes ago');
	});

	it('refuses to render a source with no ordering guarantee as a delta', () => {
		const cell = describeSync({ kind: 'full', at: '2026-08-16T09:12:00Z', ordered: false }, NOW);
		expect(cell.text).toBe('no change feed — full compare at 09:12');
	});
});

describe('timestamps carry both forms, and a date past 24 hours', () => {
	it('gives absolute plus relative inside a day', () => {
		expect(stampOf('2026-08-16T14:02:00Z', NOW)).toBe('14:02, 6 minutes ago');
	});

	it('adds the date once it is older than a day', () => {
		expect(stampOf('2026-08-15T11:47:00Z', NOW)).toBe('11:47 on 15 Aug 2026, 1 day ago');
	});

	it('formats a clock and a date the one way §9.1 allows', () => {
		expect(clockOf('2026-08-16T09:05:00Z')).toBe('09:05');
		expect(dateOf('2026-08-08T09:05:00Z')).toBe('8 Aug 2026');
	});

	it('says `just now` rather than `0 minutes ago`, and looks forward too', () => {
		expect(relativeTo('2026-08-16T14:07:40Z', NOW)).toBe('just now');
		expect(relativeTo('2026-08-16T15:08:00Z', NOW)).toBe('in 1 hour');
	});

	it('returns nothing at all for a missing or unparseable time', () => {
		expect(stampOf(undefined, NOW)).toBe('');
		expect(clockOf('not a time')).toBe('');
	});
});

describe('the three 403s are told apart on the error field', () => {
	it('maps each documented code to its own screen', () => {
		expect(classify403('sudo_required')).toBe('sudo');
		expect(classify403('forbidden')).toBe('forbidden');
		expect(classify403('csrf')).toBe('csrf');
	});

	it('treats an unknown code as `surface`, never as a retryable CSRF failure', () => {
		expect(classify403('')).toBe('other');
		expect(classify403('some_future_code')).toBe('other');
	});
});

describe('URL base normalises rather than only complaining (§17.3.1)', () => {
	it('adds the leading slash and trims the trailing one on blur', () => {
		expect(normaliseUrlBase('prowlarr/')).toEqual({ value: '/prowlarr', problem: '' });
		expect(normaliseUrlBase('  /prowlarr  ')).toEqual({ value: '/prowlarr', problem: '' });
		expect(normaliseUrlBase('/a/b///')).toEqual({ value: '/a/b', problem: '' });
	});

	it('treats empty and a bare slash as the empty default, which is valid', () => {
		expect(normaliseUrlBase('')).toEqual({ value: '', problem: '' });
		expect(normaliseUrlBase('/')).toEqual({ value: '', problem: '' });
	});

	it('errors only on what it cannot repair, and keeps what was typed', () => {
		const whole = normaliseUrlBase('https://home.tailnet.ts.net/prowlarr');
		expect(whole.problem).not.toBe('');
		expect(whole.value).toBe('https://home.tailnet.ts.net/prowlarr');
		expect(normaliseUrlBase('/prowlarr?x=1').problem).not.toBe('');
	});
});

describe('a failed connection test names the likely causes for its error class', () => {
	it('names the URL base on a 404', () => {
		const causes = likelyCauses('404 Not Found (text/html, served by nginx)', 'prowlarr');
		expect(causes.length).toBeGreaterThanOrEqual(2);
		expect(causes[0]).toContain('URL base');
	});

	it('names the pin on a TLS failure', () => {
		expect(likelyCauses('x509: certificate signed by unknown authority', 'prowlarr')[0]).toMatch(
			/fingerprint|self-signed/
		);
	});

	it('names the port on a refused connection', () => {
		expect(
			likelyCauses('dial tcp 10.0.0.4:9696: connect: connection refused', 'prowlarr')[0]
		).toContain('port');
	});

	it('names the key on a 401', () => {
		expect(likelyCauses('401 Unauthorized', 'prowlarr')[0]).toContain('API key');
	});

	it('still says something useful for an unrecognised failure', () => {
		expect(likelyCauses('something went sideways', 'prowlarr').length).toBeGreaterThan(0);
	});
});

describe('the connected panel names what actually answered', () => {
	it('joins the application, its version and its API path', () => {
		expect(connectedHeadline('Sonarr', '4.0.10.2544', '/api/v3')).toBe(
			'Sonarr 4.0.10.2544 answered on /api/v3'
		);
	});

	it('renders the bare version the *Arrs send as the PATH the user recognises', () => {
		// APIInfoResource.current is `v1` on Prowlarr and `v3` on Sonarr. `answered
		// on v1` is a version; the value the user has to match against their own
		// reverse-proxy config is `/api/v1`.
		expect(apiPathOf('v1')).toBe('/api/v1');
		expect(apiPathOf('/api/v3')).toBe('/api/v3');
		expect(apiPathOf(undefined)).toBe('');
		expect(connectedHeadline('Prowlarr', '1.37.0.5076', 'v1')).toBe(
			'Prowlarr 1.37.0.5076 answered on /api/v1'
		);
	});

	it('degrades honestly when the probe named nothing', () => {
		expect(connectedHeadline(undefined, undefined, undefined)).toBe('Connected');
		expect(connectedHeadline(undefined, undefined, '/api/v1')).toBe(
			'Something answered on /api/v1'
		);
	});
});

describe('a test never claims more than it observed', () => {
	it('splits ok:true into two outcomes on key_proven_valid', () => {
		expect(testOutcome({ ok: true, keyProvenValid: true })).toBe('connected');
		expect(testOutcome({ ok: true, keyProvenValid: false })).toBe('reachable');
		expect(testOutcome({ ok: false, keyProvenValid: false })).toBe('failed');
	});

	it('never draws an unverified credential with the confidence of a verified one', () => {
		expect(testTone('connected')).toBe('ok');
		expect(testTone('reachable')).not.toBe('ok');
		expect(testTone('failed')).toBe('err');
	});

	it('says in the title that the key was not verified', () => {
		expect(testTitle('reachable')).toContain('not verified');
		expect(testTitle('connected')).toContain('accepted');
		expect(testTitle('failed')).toBe('Could not connect');
	});
});

describe('applicationName is a lookup, because a rule would be wrong on the fourth entry', () => {
	it('capitalises the ones a rule would get wrong', () => {
		expect(applicationName('audiobookshelf')).toBe('Audiobookshelf');
		expect(applicationName('lazylibrarian')).toBe('LazyLibrarian');
	});

	it('never invents a name for a kind it does not know', () => {
		expect(applicationName('somethingelse')).toBe('somethingelse');
		expect(applicationName('')).toBe('instance');
	});
});

describe('the roll-up states a problem once and links to the row', () => {
	const rows: ServiceRow[] = [
		row({ id: 1, name: 'Prowlarr', state: 'down', breakerState: 'open', consecutiveFailures: 14 }),
		{ health: health({ id: 2, name: 'Prowlarr 2', state: 'degraded', consecutiveFailures: 1 }) },
		{ health: health({ id: 3, name: 'Prowlarr 3' }) }
	];

	it('carries only the unhealthy rows', () => {
		const entries = rollup(rows, NOW);
		expect(entries.map((e) => e.id)).toEqual([1, 2]);
	});

	it('carries no action, because the row owns the one place the fix is pressed', () => {
		for (const entry of rollup(rows, NOW)) {
			expect(Object.keys(entry).sort()).toEqual(['id', 'name', 'title', 'tone']);
		}
	});

	it('counts errors and warnings separately', () => {
		expect(rollupCount(rollup(rows, NOW))).toBe('1 error, 1 warning');
		expect(rollupCount([])).toBe('');
	});

	it('leaves a healthy install with nothing to say', () => {
		expect(rollup([row()], NOW)).toEqual([]);
	});
});
