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
		// Both are REQUIRED on ServiceHealth, so this default is what makes the
		// type a guard rather than a suggestion: drop either field from the
		// interface and every construction in this file stops compiling.
		lastFullSyncAt: null,
		workCount: 0,
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
		expect(syncCell(row(), NOW)).toMatchObject({ text: 'Not applicable', sub: 'no catalogue' });
	});

	/*
	 * ⚠️ THIS TEST WAS REVERSED, and what it used to demand is recorded because
	 * the old form is the shape of the mistake rather than an old opinion.
	 *
	 * IT USED TO READ, in full:
	 *
	 *   it('keeps `Never` for a catalogue source that has not synced', () => {
	 *     expect(syncCell(row({ kind: 'sonarr', role: 'catalogue' })).text).toBe('Never');
	 *   });
	 *
	 * It was green, it was specific, and it asserted a DEFECT as required
	 * behaviour. `syncCell` hardcoded `Never` for every catalogue row — no field
	 * was read, because none existed on the wire — so the assertion held for a
	 * row that had never synced and equally for one that synced this morning
	 * with 12,000 works behind it. A test that passes on the constant is a test
	 * that passes on the bug, and it would have passed straight through a change
	 * that declared `last_full_sync_at` on the type and then forgot to render it.
	 *
	 * WHAT CHANGED IS THE WIRE, not the taste: the endpoint now sends
	 * `last_full_sync_at` and `work_count` (internal/httpapi/services.go:655 and
	 * :660). `Never` survives as ONE of five answers, and it is now pinned to the
	 * state that earns it — a null timestamp AND a zero count — so a regression
	 * back to the constant fails on the four cases below rather than passing.
	 *
	 * It is kept and inverted rather than deleted: the `Never` word itself is
	 * still §9.1 vocabulary that a later refactor could quietly drop.
	 */
	it('says `Never` only when the timestamp is null AND the count is zero', () => {
		const cell = syncCell(row({ kind: 'sonarr', role: 'catalogue' }), NOW);
		expect(cell).toEqual({ text: 'Never', sub: '', muted: true });

		// The same row with a real timestamp must NOT still say `Never`. This is
		// the half the old assertion could not have caught.
		const synced = syncCell(
			row({ kind: 'sonarr', role: 'catalogue', lastFullSyncAt: '2026-08-16T14:02:00Z' }),
			NOW
		);
		expect(synced.text).not.toBe('Never');
		expect(synced.muted).toBe(false);
	});
});

/*
 * THE FOUR-STATE PAIR, which is the point of both fields existing.
 *
 * `last_full_sync_at` and `work_count` are read TOGETHER or not at all
 * (docs/reference/http-api.md §3.2). `null` does not mean "no data" and `0` does
 * not mean "never ran", and each of the four combinations below is a fact the
 * screen has to state differently. They are asserted as whole `Cell` objects on
 * BOTH columns, because the distinction lives in the pair of cells rather than
 * in either one.
 */
describe('the four states of last_full_sync_at x work_count', () => {
	const catalogue = (over: Partial<ServiceHealth>) =>
		row({ kind: 'sonarr', role: 'catalogue', ...over });

	it('null + 0 is `Never` and an em-dash count: nothing has been counted', () => {
		const r = catalogue({ lastFullSyncAt: null, workCount: 0 });
		expect(syncCell(r, NOW)).toEqual({ text: 'Never', sub: '', muted: true });
		expect(itemsCell(r)).toEqual({ text: '—', sub: '', muted: true });
	});

	it('a timestamp + 0 is a real `0`, because the import ran and found nothing', () => {
		const r = catalogue({ lastFullSyncAt: '2026-08-16T14:02:00Z', workCount: 0 });
		expect(syncCell(r, NOW)).toEqual({ text: '14:02', sub: '6 minutes ago', muted: false });
		// NOT `—`. An import completed; zero is the measurement it took.
		expect(itemsCell(r)).toEqual({ text: '0', sub: '', muted: false });
	});

	it('a timestamp + a count is the ordinary case', () => {
		const r = catalogue({ lastFullSyncAt: '2026-08-16T14:02:00Z', workCount: 3 });
		expect(syncCell(r, NOW)).toEqual({ text: '14:02', sub: '6 minutes ago', muted: false });
		expect(itemsCell(r)).toEqual({ text: '3', sub: '', muted: false });
	});

	/*
	 * ⚠️ THE ROW THAT FORBIDS THE OBVIOUS SIMPLIFICATION. A null timestamp with a
	 * positive count is a PARTIAL import whose committed batches stand. Treating
	 * the null as "no data" and blanking the count — the tidy-looking change —
	 * would hide rows that are really in the replica.
	 */
	it('null + a count is a partial import, and the rows are NOT hidden', () => {
		const r = catalogue({ lastFullSyncAt: null, workCount: 12 });
		expect(syncCell(r, NOW)).toEqual({
			text: 'Partial import',
			sub: 'some rows landed',
			muted: false
		});
		expect(itemsCell(r)).toEqual({ text: '12', sub: '', muted: false });
	});

	it('renders all four states differently as a pair', () => {
		const states: Array<Partial<ServiceHealth>> = [
			{ lastFullSyncAt: null, workCount: 0 },
			{ lastFullSyncAt: '2026-08-16T14:02:00Z', workCount: 0 },
			{ lastFullSyncAt: '2026-08-16T14:02:00Z', workCount: 3 },
			{ lastFullSyncAt: null, workCount: 12 }
		];
		const rendered = states.map((over) => {
			const r = catalogue(over);
			const sync = syncCell(r, NOW);
			return `${sync.text}|${sync.sub}|${itemsCell(r).text}`;
		});
		expect(new Set(rendered).size).toBe(4);
	});

	it('past a day the sync cell carries the date, matching stampOf', () => {
		const iso = '2026-08-15T11:47:00Z';
		const r = catalogue({ lastFullSyncAt: iso, workCount: 5 });
		const cell = syncCell(r, NOW);
		expect(cell.text).toBe('11:47 on 15 Aug 2026');
		expect(cell.sub).toBe('1 day ago');
		// The cell splits §9.1's pair across two slots; stampOf joins the same two
		// halves for a one-slot caller. Neither may invent its own absolute form.
		expect(stampOf(iso, NOW)).toBe(`${cell.text}, ${cell.sub}`);
	});

	/*
	 * An indexer reports null / 0 exactly like an unsynced catalogue source, so
	 * `role` is the only thing that can separate them — and it must win even when
	 * a count somehow rides along.
	 */
	it('keeps `Not applicable` for an indexer whatever the two fields say', () => {
		const r = row({ lastFullSyncAt: '2026-08-16T14:02:00Z', workCount: 9 });
		expect(syncCell(r, NOW).text).toBe('Not applicable');
		expect(itemsCell(r).text).toBe('Not applicable');
	});

	/*
	 * THE REGRESSION THIS WHOLE BLOCK EXISTS FOR. Undeclared JSON is dropped
	 * silently by the parser, so dropping `last_full_sync_at` from ServiceHealth
	 * again would not throw anywhere — every catalogue row would simply fall back
	 * to the never-synced rendering, and a suite that only checked the null case
	 * would stay green. This asserts the rendering that is IMPOSSIBLE without the
	 * field, so losing it fails here.
	 */
	it('fails if last_full_sync_at is dropped from the type again', () => {
		const r = catalogue({ lastFullSyncAt: '2026-08-16T14:02:00Z', workCount: 3 });
		const cell = syncCell(r, NOW);
		expect(cell.text).toBe('14:02');
		expect(cell.text).not.toBe('Never');
		expect(cell.muted).toBe(false);
	});

	/*
	 * And the same for the count: without `work_count` every catalogue row shows
	 * `—`, which is a defensible-looking blank rather than a crash.
	 */
	it('fails if work_count is dropped from the type again', () => {
		const r = catalogue({ lastFullSyncAt: null, workCount: 12 });
		expect(itemsCell(r).text).toBe('12');
		expect(itemsCell(r).muted).toBe(false);
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
