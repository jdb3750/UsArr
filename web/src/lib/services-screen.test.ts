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
	canRunFullSync,
	syncButtonBlocked,
	syncButtonLabel,
	syncCell,
	syncNoteWithProgress,
	syncRefusal,
	syncStarted,
	type ServiceRow
} from './services';
import type { ImportProgress, ServiceHealth } from './api';
import { NOTHING } from './list';
// The Services template AS TEXT. Vite's `?raw` is the only way to ask what a
// component contains here: vitest runs in `node` with no Svelte plugin, so the
// screen cannot be compiled, imported or rendered in this suite.
import SERVICES_SOURCE from '../routes/services/+page.svelte?raw';

/**
 * The `items` arm of the screen's cell snippet, sliced to the next arm.
 *
 * ⚠️ A MISSING MARKER THROWS RATHER THAN RETURNING ''. The failure mode of a
 * text guard is matching nothing at all: an empty corpus passes every assertion
 * over it, so a renamed column id would turn this into no check while staying
 * green.
 */
function itemsBranch(source: string): string {
	const start = source.indexOf("{:else if column.id === 'items'}");
	if (start === -1) throw new Error('the Items branch of the Services cell snippet was not found');
	const rest = source.slice(start + 1);
	const end = rest.indexOf('{:else if column.id ===');
	if (end === -1) throw new Error('the Items branch has no following branch to end at');
	return rest.slice(0, end);
}

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
		// Required as well, and the default is the SILENT value: every existing
		// assertion in this file is therefore an assertion that a clean walk adds
		// no second line to any cell.
		fileReadFailures: 0,
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

/*
 * `file_read_failures`, WHICH IS A NOTE AND NOT A FAULT.
 *
 * docs/reference/http-api.md §3.4: the number is how many DISTINCT items of this
 * instance the file walk could not read from the service, counted since the last
 * COMPLETED full sync started. A non-zero value leaves the instance healthy —
 * the import completed, the works imported and they are in the count beside the
 * note — and what is incomplete is those items' FILE facts, which is the reason
 * the availability column elsewhere reads "not counted yet" for exactly them.
 *
 * ⚠️ EVERY ASSERTION HERE IS ON A NON-ZERO VALUE OR ON THE SILENCE AT ZERO, in
 * both directions, because a test written on the default `0` passes on all three
 * ways this field is lost: dropped from the mapper, pinned to a constant, or
 * never rendered. The wire-to-cell half is in `services.test.ts`; this half is
 * the wording, the silence and the template that draws it.
 */
describe('file_read_failures on the Items cell', () => {
	const catalogue = (over: Partial<ServiceHealth>) =>
		row({ kind: 'kavita', role: 'library', ...over });

	it('says how many items could not be read, beside a count that still stands', () => {
		const r = catalogue({
			lastFullSyncAt: '2026-08-16T14:02:00Z',
			workCount: 412,
			fileReadFailures: 3
		});
		expect(itemsCell(r)).toEqual({
			text: '412',
			sub: 'File list not read for 3 items',
			muted: false
		});
	});

	/* Zero renders NOTHING, and the assertion is the whole object: a second line
	   on every healthy row is the failure this half catches. */
	it('renders no note at all at zero', () => {
		const r = catalogue({
			lastFullSyncAt: '2026-08-16T14:02:00Z',
			workCount: 412,
			fileReadFailures: 0
		});
		expect(itemsCell(r)).toEqual({ text: '412', sub: '', muted: false });
	});

	it('counts one item in the singular', () => {
		const r = catalogue({
			lastFullSyncAt: '2026-08-16T14:02:00Z',
			workCount: 9,
			fileReadFailures: 1
		});
		expect(itemsCell(r).sub).toBe('File list not read for 1 item');
	});

	/* The never-synced row still shows the note: with a null timestamp the window
	   is everything the instance holds (§3.4), and `—` beside it is the count,
	   not the note. */
	it('keeps the nothing-word and still says what could not be read', () => {
		const r = catalogue({ lastFullSyncAt: null, workCount: 0, fileReadFailures: 2 });
		expect(itemsCell(r)).toEqual({
			text: '—',
			sub: 'File list not read for 2 items',
			muted: true
		});
	});

	/* An indexer has no catalogue and no file walk; `Not applicable` is the whole
	   cell even if a count somehow rides along, exactly as it is for the pair. */
	it('says nothing on an indexer whatever the number says', () => {
		const r = row({ fileReadFailures: 5 });
		expect(itemsCell(r)).toEqual({ text: 'Not applicable', sub: '', muted: true });
	});

	/*
	 * ⚠️ IT MUST NOT TURN THE ROW RED. The State cell is written from the probe
	 * and the breaker, and this number is neither — asserted as an equality
	 * against the same row without it, so a future implementation that folded the
	 * count into the state fails here rather than shipping a red row on a healthy
	 * service.
	 */
	it('does not change the row state', () => {
		const clean = catalogue({ lastFullSyncAt: '2026-08-16T14:02:00Z', workCount: 412 });
		const failed = catalogue({
			lastFullSyncAt: '2026-08-16T14:02:00Z',
			workCount: 412,
			fileReadFailures: 3
		});
		expect(stateLabel(failed, NOW)).toEqual(stateLabel(clean, NOW));
		expect(stateLabel(failed, NOW).tone).toBe('ok');
		// And it is not an upstream problem either: `Problem` is the service's own
		// words and this note is UsArr's.
		expect(problemCell(failed)).toEqual(problemCell(clean));
	});

	/*
	 * The note is not one of §9.1's three nothing-words and must never become a
	 * fourth: those are `—`, `Not configured` and `Not applicable`, and a sentence
	 * is none of them.
	 */
	it('is a sentence, not a fourth nothing-word', () => {
		const r = catalogue({
			lastFullSyncAt: '2026-08-16T14:02:00Z',
			workCount: 412,
			fileReadFailures: 3
		});
		expect(Object.values(NOTHING)).not.toContain(itemsCell(r).sub);
	});

	/*
	 * THE TEMPLATE HALF, and it is the only half that can catch "never rendered".
	 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so the
	 * screen cannot be compiled here at all; the repo's answer to that is already
	 * in the tree — `home.test.ts` and `havecell.test.ts` read a template through
	 * Vite's `?raw` and assert over its text — and this follows it.
	 *
	 * Two failures are caught, and they are opposite:
	 *
	 *   · the branch never draws `items.sub`      → the note is computed and lost;
	 *   · it draws it OUTSIDE `{#if items.sub}`   → an empty muted line on every
	 *                                               healthy row, which is the
	 *                                               zero case rendering something.
	 */
	it('draws the note in the Items branch, and only when there is one', () => {
		const branch = itemsBranch(SERVICES_SOURCE);
		expect(branch).toContain('{items.sub}');
		// Every occurrence guarded: strip the guarded regions and nothing may be
		// left. An unguarded `{items.sub}` survives the strip and fails here.
		const unguarded = branch.replace(/\{#if items\.sub\}[\s\S]*?\{\/if\}/g, '');
		expect(unguarded).not.toContain('items.sub');
	});

	/*
	 * ⚠️ AND THE WORDING LIVES IN ONE PLACE. A template that spelled the sentence
	 * itself would drift from `itemsCell` silently, and the guard above would
	 * still pass.
	 */
	it('leaves the wording to itemsCell rather than spelling it in the markup', () => {
		expect(SERVICES_SOURCE).not.toContain('File list not read');
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

	/*
	 * THE ARM ORDER, DRILLED ON THE REAL STRING.
	 *
	 * `testBookOrbit` reports an app-info auth failure with the word "refused" in
	 * it (cmd/usarr/services.go), which the transport arm's `refused` matches. The
	 * transport arm used to run first, so all three app-info auth failures were
	 * answered with advice about the wrong port. The wrapper sentence below is
	 * verbatim from that adapter and the tail after the colon is the shape
	 * internal/bookorbit's APIError.Error() prints; only the upstream's own words
	 * at the very end stand in for text BookOrbit chooses.
	 */
	const appInfoRefusal = (upstream: string) =>
		'the magic-link token is valid and BookOrbit issued an access token, but that token was ' +
		'refused on GET /api/v1/app-info, which needs no permission at all: ' +
		upstream;

	const APP_INFO_AUTH_FAILURES = [
		[
			'a password change pending on the account',
			'bookorbit AppInfo: GET /api/v1/app-info -> 403: Password change required'
		],
		[
			'a bearer the guard rejects outright',
			'bookorbit AppInfo: GET /api/v1/app-info -> 401: Unauthorized'
		],
		[
			'an account without the reach',
			'bookorbit AppInfo: GET /api/v1/app-info -> 403: insufficient permissions'
		]
	] as const;

	it('answers every app-info auth failure with auth advice, never with the port', () => {
		for (const [what, upstream] of APP_INFO_AUTH_FAILURES) {
			const causes = likelyCauses(appInfoRefusal(upstream), 'bookorbit');
			// ⚠️ THE ABSENCE ASSERTION IS GUARDED. `[]` and `['']` both satisfy
			// "does not mention the port", and both would be a broken panel.
			expect(causes.length, what).toBeGreaterThan(0);
			for (const cause of causes) expect(cause.trim(), what).not.toBe('');
			const joined = causes.join(' ');
			expect(joined, what).not.toContain('The wrong port');
			expect(joined, what).toContain('magic-link token');
		}
	});

	it('keeps the port arm firing for what it exists for, and only for that', () => {
		// The shapes that actually reach here, all of them through the pinned
		// dialer, so all of them carrying net.OpError's prefix (internal/ssrf/dial.go).
		const transport = [
			'get "http://prowlarr.lan:9696/api/v1/system/status": ssrf: dial prowlarr.lan:9696: dial tcp 10.0.0.4:9696: connect: connection refused',
			'dial tcp: lookup prowlarr.lan: no such host',
			'context deadline exceeded (Client.Timeout exceeded while awaiting headers)',
			'dial tcp 10.0.0.4:9696: connect: network is unreachable'
		];
		for (const message of transport) {
			const causes = likelyCauses(message, 'prowlarr');
			expect(causes.length, message).toBeGreaterThan(0);
			expect(causes[0], message).toContain('The wrong port');
		}
		// The other direction: an arm that fires on everything is no arm. A failure
		// with no transport shape must reach the default advice instead.
		const unrecognised = likelyCauses('something went sideways', 'prowlarr');
		expect(unrecognised.length).toBeGreaterThan(0);
		for (const cause of unrecognised) expect(cause.trim()).not.toBe('');
		expect(unrecognised.join(' ')).not.toContain('The wrong port');
	});

	it('does not read a port that happens to contain 401 as an auth failure', () => {
		// What moving the auth arm up would otherwise cost: a bare /401/ matches
		// inside `:4013`, and that message is a dial failure, not a refused key.
		const causes = likelyCauses('dial tcp 10.0.0.4:4013: connect: connection refused', 'prowlarr');
		expect(causes[0]).toContain('The wrong port');
	});

	it('still names the key on a plain 401 from an *Arr', () => {
		expect(
			likelyCauses(
				'servarr SystemStatus: GET /api/v3/system/status -> 401: Unauthorized',
				'sonarr'
			)[0]
		).toContain('API key');
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

/**
 * RUN FULL SYNC NOW — http-api.md §4, and the three things a client gets wrong.
 *
 *  1. It renders 202 as "imported". The response is written before anything has
 *     been read, so `started` is the strongest word available and completion is
 *     observed on a different endpoint entirely.
 *  2. It switches on the status code. Three separate 409s arrive here with
 *     opposite fixes, so a status branch shows the wrong sentence two times in
 *     three.
 *  3. It decides "can this service sync" from the freshness fields. A Prowlarr
 *     reports null / 0 exactly like a catalogue source that has never imported,
 *     and only `role` separates them.
 */
describe('the re-import control is offered on `role`, never on the freshness pair', () => {
	it('offers it on a catalogue source that has never synced', () => {
		expect(canRunFullSync(health({ kind: 'kavita', role: 'library' }))).toBe(true);
	});

	/* THE ALREADY-SYNCED POPULATION, which is every install after its first
	   import and which the three cases around this one do not reach: all of
	   them construct the default row, whose freshness pair is `null` / `0`. An
	   implementation that read `lastFullSyncAt === null` — "offer it only until
	   the first import lands" — would satisfy every one of them and would hide
	   the button on exactly the installs that press it. This case is the one
	   that fails when that happens. */
	it('offers it on a catalogue source that HAS synced, with rows already imported', () => {
		const synced = health({
			kind: 'kavita',
			role: 'library',
			lastFullSyncAt: '2026-08-16T09:00:00Z',
			workCount: 4821
		});
		// Stated as the contrast, not just as a true assertion: this row differs
		// from the never-synced one in both freshness fields and in neither of
		// the fields `canRunFullSync` is allowed to read.
		const unsynced = health({ kind: 'kavita', role: 'library' });
		expect(synced.lastFullSyncAt).not.toBe(unsynced.lastFullSyncAt);
		expect(synced.workCount).not.toBe(unsynced.workCount);
		expect(canRunFullSync(synced)).toBe(true);
	});

	it('refuses it for an indexer whose two fields look identical to that', () => {
		const indexer = health({ kind: 'prowlarr', role: 'indexer' });
		const unsynced = health({ kind: 'kavita', role: 'library' });
		expect(indexer.lastFullSyncAt).toBe(unsynced.lastFullSyncAt);
		expect(indexer.workCount).toBe(unsynced.workCount);
		expect(canRunFullSync(indexer)).toBe(false);
		expect(canRunFullSync(unsynced)).toBe(true);
	});

	it('still offers it on a DISABLED catalogue source, so the server can say why', () => {
		expect(canRunFullSync(health({ kind: 'kavita', role: 'library', enabled: false }))).toBe(true);
	});
});

describe('202 is `started`, and no branch may say anything stronger', () => {
	it('names the instance and says what completion actually looks like', () => {
		const note = syncStarted({ name: 'Kavita' });
		expect(note.phase).toBe('started');
		expect(note.tone).toBe('ok');
		expect(note.title).toBe('Kavita is re-importing');
		expect(note.consequence).toContain('Last successful sync');
	});

	it('claims no progress and no completion anywhere in its copy', () => {
		const words = Object.values(syncStarted({ name: 'Kavita' }))
			.join(' ')
			.toLowerCase();
		for (const banned of ['imported', 'complete', 'finished importing', '%', 'progress bar']) {
			expect(words).not.toContain(banned);
		}
	});

	it('falls back to a nameless headline rather than rendering an empty name', () => {
		expect(syncStarted({ name: '' }).title).toBe('A full re-import has started');
	});
});

describe('the three 409s are told apart on `error`, and only one is not a refusal', () => {
	const conflict = (code: string) =>
		syncRefusal(409, code, 'the server said this', 'the server said that');

	it('reads import_in_progress as ALREADY RUNNING, not as a failure', () => {
		const note = conflict('import_in_progress');
		expect(note.phase).toBe('running');
		expect(note.tone).not.toBe('err');
		expect(note.consequence).toContain('disturbed nothing');
		// The one code that must NOT claim the catalogue was left alone by
		// everything: another import is writing to it right now.
		expect(note.consequence).not.toContain('untouched');
	});

	it('reads not_a_catalogue_source as a refusal that started nothing', () => {
		const note = conflict('not_a_catalogue_source');
		expect(note.phase).toBe('refused');
		expect(note.consequence).toContain('No import started');
	});

	it('locates the missing catalogue reader in UsArr, never in the service', () => {
		// The server's sentence renders directly beneath this title and reads
		// "UsArr has no catalogue reader for this service", so a headline claiming
		// the service has no catalogue contradicted the line under it. What made
		// that visible was a BookOrbit, which registers as `role: library` and so
		// renders the Sync button; it has an adapter now, and the rule does not
		// depend on it — the code is shown to whichever kind has no adapter next.
		const note = syncRefusal(
			409,
			'not_a_catalogue_source',
			'UsArr has no catalogue reader for this service',
			'Run a sync on a service UsArr can import from'
		);
		// ⚠️ GUARDED: an empty title satisfies every `not.toMatch` below.
		expect(note.title.trim()).not.toBe('');
		expect(note.title).toContain('UsArr');
		expect(note.title).not.toMatch(/no catalogue|has no catalogue|nothing to import/i);
		// And the pair must not disagree, which is the whole defect.
		expect(note.message).toContain('no catalogue reader');
	});

	it('reads service_disabled as its own sentence, not as the other two', () => {
		const disabled = conflict('service_disabled');
		expect(disabled.title).not.toBe(conflict('not_a_catalogue_source').title);
		expect(disabled.title).not.toBe(conflict('import_in_progress').title);
	});

	it('gives all three the same status and three different renderings', () => {
		const titles = ['import_in_progress', 'not_a_catalogue_source', 'service_disabled'].map(
			(c) => conflict(c).title
		);
		expect(new Set(titles).size).toBe(3);
	});

	it('carries the server`s own message and action through untouched', () => {
		const note = conflict('service_disabled');
		expect(note.message).toBe('the server said this');
		expect(note.action).toBe('the server said that');
	});
});

describe('404, 501 and 500 are separated: only the last one is a fault', () => {
	it('treats a 404 and a 501 as refusals rather than as errors', () => {
		expect(syncRefusal(404, 'not_found', 'gone', '').tone).not.toBe('err');
		expect(syncRefusal(501, 'not_configured', 'no importer', '').tone).not.toBe('err');
	});

	it('treats a 500 as the fault it is, and still says nothing started', () => {
		const note = syncRefusal(500, 'internal', 'the credential would not open', 'Test connection');
		expect(note.phase).toBe('not_started');
		expect(note.tone).toBe('err');
		expect(note.message).toBe('the credential would not open');
		expect(note.consequence).toContain('No import started');
	});

	it('falls into the fault branch for a code it has never seen', () => {
		expect(syncRefusal(418, 'brand_new_code', 'x', '').phase).toBe('not_started');
	});
});

describe('the button says which of the two waits it is in', () => {
	it('separates UsArr waiting on its own server from another import owning the row', () => {
		expect(syncButtonLabel('starting')).toBe('Starting');
		expect(syncButtonLabel('running')).toBe('Import already running');
		expect(syncButtonLabel('starting')).not.toBe(syncButtonLabel('running'));
	});

	it('returns to the plain label after every settled outcome', () => {
		for (const phase of ['idle', 'started', 'refused', 'not_started', 'stopped'] as const) {
			expect(syncButtonLabel(phase)).toBe('Run full sync now');
			expect(syncButtonBlocked(phase)).toBe(false);
		}
	});

	it('is unavailable while starting and while another import is running', () => {
		expect(syncButtonBlocked('starting')).toBe(true);
		expect(syncButtonBlocked('running')).toBe(true);
	});
});

/**
 * THE STREAM FOLDED ONTO THE PRESS, and mostly the four cases where it must
 * refuse to say anything new.
 *
 * The constraint that outranks the feature: a readout that stops moving is
 * worse than no readout, because the user reads a frozen count as a frozen
 * import. So the numbers exist only while the socket is up, and the only
 * sentence that ends an import is the one the server sent.
 */
describe('the import stream folded onto the note', () => {
	const frame = (over: Partial<ImportProgress> = {}): ImportProgress => ({
		instanceId: 4,
		phase: 'items',
		itemsRead: 0,
		applied: 0,
		...over
	});
	const started = () => syncStarted({ name: 'Kavita' });

	it('says exactly what it said before, when no frame has arrived', () => {
		const before = started();
		expect(syncNoteWithProgress(before, undefined, true)).toEqual(before);
		// And identically when the socket never opened at all.
		expect(syncNoteWithProgress(before, undefined, false)).toEqual(before);
	});

	it('reports the container phase as read but not applied', () => {
		const note = syncNoteWithProgress(started(), frame({ phase: 'containers' }), true);
		expect(note.consequence).toContain('Reading the list of libraries');
		expect(note.phase).toBe('started');
	});

	it('reports real item counts, with no denominator the server did not send', () => {
		const note = syncNoteWithProgress(started(), frame({ itemsRead: 1200, applied: 1000 }), true);
		expect(note.consequence).toContain('Read 1,200 items so far, and applied 1,000 of them.');
		expect(note.consequence).not.toContain(' of 0');
		expect(note.consequence).not.toContain('%');
	});

	it('uses a denominator only in the phase that sends one', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'credits', itemsRead: 40, applied: 118, total: 400 }),
			true
		);
		expect(note.consequence).toContain('40 of 400 items read');
		expect(note.consequence).toContain('118 credits written');
	});

	it('never claims completion from a count that reached its total', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'credits', itemsRead: 400, applied: 400, total: 400 }),
			true
		);
		expect(note.phase).toBe('started');
		expect(note.title).toBe('Kavita is re-importing');
		expect(note.consequence).toContain('Last successful sync');
	});

	it('ends the import on the terminal frame and on nothing else', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'done', itemsRead: 1200, applied: 1200 }),
			true
		);
		expect(note.phase).toBe('finished');
		expect(note.title).toBe('The import has finished');
		expect(note.consequence).toContain('read 1,200 items and applied 1,200');
		// A finished import releases the button again.
		expect(syncButtonBlocked(note.phase)).toBe(false);
		expect(syncButtonLabel(note.phase)).toBe('Run full sync now');
	});

	it('withdraws the counts when the socket drops mid import, rather than freezing them', () => {
		const live = syncNoteWithProgress(started(), frame({ itemsRead: 900, applied: 900 }), true);
		const dropped = syncNoteWithProgress(started(), frame({ itemsRead: 900, applied: 900 }), false);
		expect(live.consequence).toContain('900');
		expect(dropped.consequence).not.toContain('900');
		expect(dropped.consequence).toContain('not connected');
		expect(dropped.consequence).toContain('Last successful sync');
		// AND IT IS NOT AN ENDING. A drop says unknown, never finished.
		expect(dropped.phase).toBe('started');
	});

	it('drops a wait instruction that the completion has made false', () => {
		const running = syncRefusal(409, 'import_in_progress', 'already running', 'Wait for it');
		const note = syncNoteWithProgress(
			running,
			frame({ phase: 'done', itemsRead: 9, applied: 9 }),
			true
		);
		expect(note.title).toBe('The import has finished');
		// Verbatim means never PARAPHRASED. A live instruction to wait for
		// something that is over is not the server's current word on anything.
		expect(note.message).toBe('');
		expect(note.action).toBe('');
	});

	it('keeps a completion that was observed, whatever the socket does afterwards', () => {
		const done = frame({ phase: 'done', itemsRead: 12, applied: 12 });
		expect(syncNoteWithProgress(started(), done, false).phase).toBe('finished');
	});

	it('shows the progress of an import that was already running when the press was refused', () => {
		const running = syncRefusal(409, 'import_in_progress', 'already running', 'Wait');
		const note = syncNoteWithProgress(running, frame({ itemsRead: 5, applied: 5 }), true);
		expect(note.consequence).toContain('Read 5 items');
		expect(note.title).toBe('An import is already running');
	});

	it('leaves a refusal and a fault entirely alone, whatever the stream says', () => {
		for (const refused of [
			syncRefusal(409, 'service_disabled', 'off', 'Enable the service'),
			syncRefusal(500, 'internal', 'the credential would not open', 'Test connection')
		]) {
			expect(syncNoteWithProgress(refused, frame({ phase: 'done' }), true)).toEqual(refused);
		}
	});

	/**
	 * http-api.md §5.4, pinned with the numbers that make it fail.
	 *
	 * In the credits phase the two fields are in DIFFERENT UNITS: `applied`
	 * counts credit ROWS and `total` counts credit REQUESTS, i.e. items. One
	 * item can carry several credits, so `applied` can exceed `total` and their
	 * ratio is not a fraction of anything. These are the recorded numbers from
	 * `__fixtures__/sse-frames.json` — a real run of two series where one had
	 * three credited people and the other had none.
	 *
	 * The earlier denominator test uses 118 against 400, which would pass just
	 * as happily against code that divided one by the other. This one would not.
	 */
	it('pairs the denominator with the items read, never with the credits written', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'credits', itemsRead: 2, applied: 3, total: 2 }),
			true
		);
		expect(note.consequence).toContain('2 of 2 items read');
		expect(note.consequence).toContain('3 credits written');
		// The two ways a fraction would show itself, and a percentage of 150%.
		expect(note.consequence).not.toContain('3 of 2');
		expect(note.consequence).not.toContain('3/2');
		expect(note.consequence).not.toContain('%');
		// And a count above its total is still no kind of ending.
		expect(note.phase).toBe('started');
	});

	/**
	 * THE FILES PHASE, whose fields do NOT mean what the same fields mean in
	 * `items`, measured at the publish site rather than assumed from the names.
	 *
	 * `internal/libsync/importer.go:608-612` publishes, per committed batch:
	 *
	 *   items_read  rep.FileItemsRead  — items the walk got volumes for (:623)
	 *   applied     rep.Files.FilesWritten — file ROWS inserted or updated,
	 *               which store/files.go:104 is explicit is a count of work
	 *               done and NOT a count of new files
	 *   total       len(reqs)          — the items being walked, one request
	 *               each
	 *
	 * So it is the credits case exactly: `applied` and `total` are in different
	 * units, one item carries as many files as it has, and `applied / total` is
	 * a fraction of nothing. The denominator pairs with the items read.
	 */
	it('gives the files phase a sentence of its own, rather than the stranger fallback', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'files', itemsRead: 40, applied: 1180, total: 400 }),
			true
		);
		expect(note.consequence, 'the files phase is not taking its own arm').toContain(
			'Recording files: 40 of 400 items read, 1,180 files recorded.'
		);
		// The default arm's exact words, which `files` must no longer reach.
		expect(
			note.consequence,
			'the files phase fell through to the unnamed default arm'
		).not.toContain('Read 40 so far, and applied 1,180.');
		// It is progress, not an ending, and not a claim about the library.
		expect(note.phase).toBe('started');
		expect(note.consequence).not.toContain('missing');
		expect(note.consequence).not.toContain('complete');
	});

	/**
	 * The files twin of the credits fraction pin, with numbers that only pass
	 * against code that keeps the two units apart: one item, one credit-free
	 * series, three files under it.
	 */
	it('pairs the files denominator with the items read, never with the files recorded', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'files', itemsRead: 2, applied: 3, total: 2 }),
			true
		);
		expect(note.consequence).toContain('2 of 2 items read');
		expect(note.consequence).toContain('3 files recorded');
		expect(note.consequence).not.toContain('3 of 2');
		expect(note.consequence).not.toContain('3/2');
		expect(note.consequence).not.toContain('%');
		expect(note.phase).toBe('started');
	});

	/**
	 * `total` is `omitempty`, so a bare count is the shape a client has to hold
	 * whatever the current publisher happens to send. Never ` of 0`, which
	 * would read the absent total as a source that reported zero.
	 */
	it('renders the files phase without a denominator the frame did not carry', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'files', itemsRead: 40, applied: 1180 }),
			true
		);
		expect(note.consequence).toContain('Recording files: 40 items read, 1,180 files recorded.');
		expect(note.consequence).not.toContain(' of 0');
	});

	/**
	 * All five phases in the order a full run sends them, read as a sequence
	 * because the sentence has to sit beside its neighbours without the line
	 * changing shape under the reader.
	 */
	it('walks containers, items, credits, files and the terminal frame in order', () => {
		const press = started();
		const screen = (last: ImportProgress) => syncNoteWithProgress(press, last, true);

		expect(screen(frame({ phase: 'containers' })).consequence).toContain(
			'Reading the list of libraries. Nothing has been applied yet.'
		);
		expect(screen(frame({ phase: 'items', itemsRead: 400, applied: 400 })).consequence).toContain(
			'Read 400 items so far, and applied 400 of them.'
		);
		expect(
			screen(frame({ phase: 'credits', itemsRead: 400, applied: 118, total: 400 })).consequence
		).toContain('Adding credits: 400 of 400 items read, 118 credits written.');
		expect(
			screen(frame({ phase: 'files', itemsRead: 400, applied: 1180, total: 400 })).consequence
		).toContain('Recording files: 400 of 400 items read, 1,180 files recorded.');

		// Only the terminal frame ends it, and the files counts do not survive
		// into it: `done` reports the catalogue pass, which is a different count.
		const done = screen(frame({ phase: 'done', itemsRead: 400, applied: 400 }));
		expect(done.phase).toBe('finished');
		expect(done.consequence).toContain('It read 400 items and applied 400.');
	});

	/**
	 * http-api.md §5.2, pinned as a SEQUENCE rather than as one frame.
	 *
	 * `credits` is skipped whole for a source that reports none or is not a
	 * CreditSource (internal/libsync/importer.go:415-418), so THREE frames is a
	 * complete healthy run. The risk this covers is a client that treats the
	 * four phases as an ordered scale and waits for a fourth.
	 *
	 * Derived the way the screen derives it: `syncNotes` keeps what the server
	 * answered the press with, the stream contributes the LAST frame only, and
	 * the sentence is recomputed from both on every read.
	 */
	it('finishes a run that skipped the credits phase entirely', () => {
		const press = started();
		const screen = (last: ImportProgress) => syncNoteWithProgress(press, last, true);

		const containers = screen(frame({ phase: 'containers' }));
		expect(containers.phase).toBe('started');
		expect(containers.consequence).toContain('Reading the list of libraries');

		const items = screen(frame({ phase: 'items', itemsRead: 2, applied: 2 }));
		expect(items.phase).toBe('started');
		expect(items.consequence).toContain('Read 2 items so far, and applied 2 of them.');

		// No credits frame. The next thing the server sends is the terminal one.
		const done = screen(frame({ phase: 'done', itemsRead: 2, applied: 2 }));
		expect(done.phase).toBe('finished');
		expect(done.title).toBe('The import has finished');
		expect(done.consequence).toContain('It read 2 items and applied 2.');
		expect(done.consequence).toContain('Last successful sync has moved');
		// Nothing is still pending: the button is released and says so.
		expect(syncButtonBlocked(done.phase)).toBe(false);
		expect(syncButtonLabel(done.phase)).toBe('Run full sync now');
	});

	it('renders a phase it has never heard of as counts, rather than dropping it', () => {
		const note = syncNoteWithProgress(
			started(),
			frame({ phase: 'reconciling', itemsRead: 3, applied: 2 }),
			true
		);
		expect(note.consequence).toContain('Read 3 so far, and applied 2.');
		expect(note.phase).toBe('started');
	});
});

/**
 * http-api.md §5.5's terminal stream frame, and the four properties that make
 * it different from every other frame this client handles.
 *
 * ⚠️ NOTHING EMITS IT YET. §5.5's own 🚩 says so, and §5.1's shipped behaviour
 * — a failed import publishes nothing at all — still holds beside it. So the
 * first thing pinned here is that a `stopped` which never arrives changes
 * nothing, which is what makes this arm safe to land ahead of its producer.
 */
describe('the terminal `stopped` frame, §5.5', () => {
	const frame = (over: Partial<ImportProgress> = {}): ImportProgress => ({
		instanceId: 4,
		phase: 'stopped',
		itemsRead: 41,
		applied: 40,
		...over
	});
	const started = () => syncStarted({ name: 'Kavita' });

	it('says the import stopped, and how much of it stands', () => {
		const note = syncNoteWithProgress(started(), frame(), true);
		expect(note.phase).toBe('stopped');
		expect(note.title).toBe('The import stopped before it finished');
		expect(note.consequence).toContain('It read 41 items and applied 40.');
		expect(note.consequence).toContain('committed rather than rolled back');
	});

	/**
	 * §5.5.4, as copy. It has three jobs and dropping any one of them is
	 * misreporting: say it stopped, say how much stands, and say the catalogue
	 * is not known to be complete. Never "imported", never a tick, never a
	 * count presented as a result.
	 */
	it('reads as neither a success nor a confirmed failure', () => {
		const note = syncNoteWithProgress(started(), frame(), true);
		const words = `${note.title} ${note.consequence}`.toLowerCase();
		// "stopped before it finished" is honest and stays; what may not appear
		// is any claim that it DID finish, import, or complete.
		for (const banned of ['imported', 'has finished', 'is complete', 'succeeded', '%']) {
			expect(words).not.toContain(banned);
		}
		expect(note.title).not.toBe('The import has finished');
		expect(note.consequence).toContain('not known to be complete');
		// `warn`, not `err`: with no cause the frame cannot assert a fault.
		expect(note.tone).toBe('warn');
		expect(note.tone).not.toBe('ok');
	});

	/**
	 * §5.5.2, which is the whole reason `failed` was renamed. The refusal's
	 * sentence is the exact negation of this frame's meaning: a stop means the
	 * import DID start and DID run, and §5.5.4's committed batches STAND.
	 */
	it('never reuses the refusal`s "the catalogue is untouched" sentence', () => {
		const stopped = syncNoteWithProgress(started(), frame(), true);
		const notStarted = syncRefusal(500, 'internal', 'the credential would not open', 'Test');
		expect(stopped.consequence).not.toContain('untouched');
		expect(stopped.consequence).not.toContain('No import started');
		expect(notStarted.consequence).toContain('No import started');
		// One word may not carry both claims, so the two phases differ too.
		expect(stopped.phase).not.toBe(notStarted.phase);
		expect(notStarted.phase).toBe('not_started');
		expect(stopped.title).not.toBe(notStarted.title);
	});

	/**
	 * §5.5.5. One phase value, zero new fields, and no `reason` anywhere. The
	 * client renders nothing about why, because there is nothing to render and
	 * inventing one from the counters is how a disk error is reported as a
	 * broken API key.
	 */
	it('says nothing at all about why, and carries no upstream text', () => {
		const running = syncRefusal(409, 'import_in_progress', 'already running', 'Wait for it');
		const note = syncNoteWithProgress(running, frame(), true);
		// The server`s own words are dropped, exactly as `done` drops them: an
		// instruction to wait for a run that has stopped is not current.
		expect(note.message).toBe('');
		expect(note.action).toBe('');
		const words = `${note.title} ${note.consequence}`.toLowerCase();
		for (const guess of ['api key', 'unreachable', 'refused', 'timed out', 'credential']) {
			expect(words).not.toContain(guess);
		}
		expect(note.consequence).toContain('does not say why');
	});

	/**
	 * §5.5.4 and §5.5.3 together. `last_full_sync_at` is written after
	 * `rep.Completed = true` and on success alone, and the frame is losable
	 * (`Hub.Publish` drops a full subscriber), so §3's column stays the
	 * authority. The copy points AT the column rather than reporting a value.
	 */
	it('leaves the health column as the authority rather than reporting it', () => {
		const note = syncNoteWithProgress(started(), frame(), true);
		expect(note.consequence).toContain('Last successful sync does not move on a stop');
		// Never a claim about what the column now READS, which is the health
		// read`s answer and not this frame`s.
		expect(note.consequence).not.toContain('has moved');
		expect(note.consequence).not.toContain('is empty');
	});

	it('reports a stop that read nothing without pretending it read something', () => {
		const note = syncNoteWithProgress(started(), frame({ itemsRead: 0, applied: 0 }), true);
		expect(note.phase).toBe('stopped');
		expect(note.title).toBe('The import stopped before it finished');
		expect(note.consequence).toContain('It read 0 items and applied 0.');
		expect(note.consequence).toContain('not known to be complete');
	});

	/**
	 * §5.5.3's first ⚠️. Silence, a drop, a reconnect and a `stream.missed`
	 * cannot downgrade a stop this client has already seen: silence is UNKNOWN
	 * (§5.1) and unknown never overwrites a fact. So the branch sits ABOVE the
	 * `!connected` arm, exactly as `done` does.
	 */
	it('keeps a stop that was observed, whatever the socket does afterwards', () => {
		const dropped = syncNoteWithProgress(started(), frame(), false);
		expect(dropped.phase).toBe('stopped');
		expect(dropped.title).toBe('The import stopped before it finished');
		// And it does NOT fall into the disconnected sentence.
		expect(dropped.consequence).not.toContain('not connected');
		expect(dropped.consequence).toContain('It read 41 items and applied 40.');
	});

	/**
	 * §5.5.3's second ⚠️, AND THE ONE PROPERTY A CLIENT IS MOST LIKELY TO GET
	 * WRONG. The frame carries `instance_id` and NO run id, unlike `search.*`
	 * with its `search_id`. So a later non-terminal frame for that instance is
	 * a SECOND RUN starting, never a retraction of the first stop.
	 *
	 * Derived the way the screen derives it: `syncNotes` keeps what the server
	 * answered the press with, the stream contributes the LAST frame only, and
	 * the sentence is recomputed from both on every read.
	 */
	it('reads a later `items` frame as a new run, not as the stop being undone', () => {
		const press = started();
		const screen = (last: ImportProgress) => syncNoteWithProgress(press, last, true);

		const stopped = screen(frame({ itemsRead: 41, applied: 40 }));
		expect(stopped.phase).toBe('stopped');

		// A second run begins. Its counters start over, and 3 is BELOW the 41
		// the stop reported, which is exactly what a retraction-shaped
		// implementation (keeping the highest count, or latching terminal) would
		// hide.
		const again = screen(frame({ phase: 'items', itemsRead: 3, applied: 2 }));
		expect(again.phase).toBe('started');
		expect(again.title).toBe('Kavita is re-importing');
		expect(again.consequence).toContain('Read 3 items so far, and applied 2 of them.');
		expect(again.consequence).not.toContain('stopped');
		expect(again.consequence).not.toContain('41');
		// The button follows the phase back to in-progress wording`s absence:
		// `started` was never a blocked phase, and neither is `stopped`.
		expect(syncButtonBlocked(stopped.phase)).toBe(false);
		expect(syncButtonBlocked(again.phase)).toBe(false);

		// And a `containers` frame does the same, which is the bootstrap path.
		const bootstrap = screen(frame({ phase: 'containers', itemsRead: 0, applied: 0 }));
		expect(bootstrap.phase).toBe('started');
		expect(bootstrap.consequence).toContain('Reading the list of libraries');
	});

	it('releases the button, because the run it describes is over', () => {
		const note = syncNoteWithProgress(started(), frame(), true);
		expect(syncButtonBlocked(note.phase)).toBe(false);
		expect(syncButtonLabel(note.phase)).toBe('Run full sync now');
	});

	it('leaves a refusal and a fault alone, exactly as `done` is left', () => {
		for (const refused of [
			syncRefusal(409, 'service_disabled', 'off', 'Enable the service'),
			syncRefusal(500, 'internal', 'the credential would not open', 'Test connection')
		]) {
			expect(syncNoteWithProgress(refused, frame(), true)).toEqual(refused);
		}
	});

	/**
	 * §5.5 is a contract for a producer that does not exist. The arm has to be
	 * a no-op until one lands, and that is not an assumption: it is the same
	 * "no frame, no change" refusal the rest of this file pins, restated for
	 * the phase that will never arrive on today's server.
	 */
	it('changes nothing at all while no producer emits the frame', () => {
		const before = started();
		expect(syncNoteWithProgress(before, undefined, true)).toEqual(before);
		expect(syncNoteWithProgress(before, undefined, false)).toEqual(before);
		// And a healthy run still ends the one way it always did.
		const done = syncNoteWithProgress(before, frame({ phase: 'done' }), true);
		expect(done.phase).toBe('finished');
	});
});
