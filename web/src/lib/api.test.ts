import { describe, expect, it } from 'vitest';
import {
	STREAM_EVENT_NAMES,
	normalizeStreamEvent,
	problemsFrom,
	toIndexerOutcome,
	toRecentGrab,
	toRelease,
	toReport,
	type StreamEvent
} from './api';
import { formatAge, formatSize, protocolClass } from './format';
import recorded from './__fixtures__/sse-frames.json';

/**
 * The event-name contract, pinned from the client side.
 *
 * internal/httpapi/events_test.go pins the SAME literal strings from the server
 * side (TestEventNamesAreTheWireContract). Renaming an event on either side
 * fails one of the two — which is the failure that did not happen when the
 * client listened for `search.result`/`search.status` and the server had always
 * emitted `search.results`/`search.indexer`.
 */
const SERVER_EVENT_NAMES = [
	'search.started',
	'search.indexer',
	'search.results',
	'search.done',
	'search.failed',
	'stream.missed'
];

describe('the SSE event-name contract', () => {
	it('is exactly the set internal/httpapi emits', () => {
		expect([...STREAM_EVENT_NAMES]).toEqual(SERVER_EVENT_NAMES);
	});

	it('resolves every one of those names to something other than unknown', () => {
		// A minimal but structurally valid payload per name: the point is that the
		// NAME is recognised, so nothing here may be rejected on its fields.
		const minimal: Record<string, unknown> = {
			'search.started': { search_id: 's1' },
			'search.indexer': { search_id: 's1', phase: 'indexer_done', indexer: { indexer_id: 1 } },
			'search.results': { search_id: 's1', results: [] },
			'search.done': { search_id: 's1', report: {} },
			'search.failed': { search_id: 's1', message: 'out of time' },
			'stream.missed': { message: 'events were dropped' }
		};
		for (const name of STREAM_EVENT_NAMES) {
			const event = normalizeStreamEvent(name, JSON.stringify(minimal[name]));
			expect(event.kind, `${name} must be understood`).not.toBe('unknown');
		}
	});

	it('does not answer to a name the server does not send', () => {
		// The two the client used to listen for. They were never on the wire.
		expect(normalizeStreamEvent('search.result', '{"release":{"id":"r1","title":"x"}}').kind).toBe(
			'unknown'
		);
		expect(normalizeStreamEvent('search.status', '{"problems":[]}').kind).toBe('unknown');
		// The server always writes an `event:` line, so a default message frame
		// carrying a `type` field is not a shape this client guesses at any more.
		expect(normalizeStreamEvent('message', '{"type":"search.results","results":[]}').kind).toBe(
			'unknown'
		);
	});
});

/**
 * The frames in __fixtures__/sse-frames.json were captured off the real server
 * by cmd/usarr's end-to-end test (TestSSEFramesMatchTheClientContract writes the
 * same shapes it asserts). Replaying them here is the assertion that would
 * actually have caught the bug: it exercises names AND field spellings together.
 */
describe('replaying recorded server frames', () => {
	const frames = recorded as unknown as { name: string; data: unknown }[];
	const events = frames.map((frame) =>
		normalizeStreamEvent(frame.name, JSON.stringify(frame.data))
	);

	it('understands every recorded frame', () => {
		const unknown = events
			.map((event, i) => ({ kind: event.kind, name: frames[i].name }))
			.filter(({ kind }) => kind === 'unknown');
		expect(unknown).toEqual([]);
	});

	it('renders the releases the server actually sent', () => {
		const releases = events
			.filter((e): e is Extract<StreamEvent, { kind: 'results' }> => e.kind === 'results')
			.flatMap((e) => e.releases);
		expect(releases.length).toBeGreaterThan(0);
		expect(releases[0]).toMatchObject({
			candidateId: 1,
			guid: 'https://tracker.example/details/1234',
			title: 'Test.Release.2026.1080p.WEB-DL.x264-USARR',
			indexerName: 'Working Tracker',
			protocol: 'torrent',
			sizeBytes: 2147483648,
			seeders: 42
		});
		// The size and the age are the two the page renders through a formatter,
		// so a retagged field would show as a blank cell rather than an error.
		expect(formatSize(releases[0].sizeBytes)).toBe('2.0 GiB');
		expect(formatAge(releases[0].ageDays)).toBe('2 d');
		// Prowlarr embeds its admin key in downloadUrl/magnetUrl, so neither is on
		// the wire and neither may creep back into the client shape.
		expect(Object.keys(releases[0])).not.toContain('downloadUrl');
		expect(Object.keys(releases[0])).not.toContain('magnetUrl');
	});

	it('tracks each indexer as it reports', () => {
		const indexers = events.filter(
			(e): e is Extract<StreamEvent, { kind: 'indexer' }> => e.kind === 'indexer'
		);
		expect(indexers.length).toBeGreaterThan(0);
		expect(indexers.map((e) => e.phase)).toContain('indexer_started');
		expect(indexers.map((e) => e.phase)).toContain('indexer_done');
		expect(indexers.find((e) => e.indexer.answered)?.indexer).toMatchObject({
			indexerId: 1,
			name: 'Working Tracker',
			status: 'ok',
			count: 1
		});
	});

	it('closes with a report that names what did not answer', () => {
		const done = events.find((e): e is Extract<StreamEvent, { kind: 'done' }> => e.kind === 'done');
		expect(done?.report).toMatchObject({
			degraded: true,
			totalIndexers: 3,
			answered: 1,
			skipped: 2,
			results: 1
		});
		expect(done?.report?.summary).toContain('indexers answered');
		expect(problemsFrom(done?.report?.indexers ?? [])).toContainEqual({
			indexer: 'Disabled Tracker',
			error: 'indexer is disabled in Prowlarr'
		});
		// The indexer that DID answer is not a problem.
		expect(problemsFrom(done?.report?.indexers ?? []).map((p) => p.indexer)).not.toContain(
			'Working Tracker'
		);
	});
});

describe('toRelease', () => {
	it('reads the server field names and nothing else', () => {
		expect(
			toRelease({
				candidate_id: 7,
				guid: 'g',
				title: 'Some.Release.2024',
				indexer_name: 'nzbgeek',
				protocol: 'usenet',
				size_bytes: 1024,
				seeders: 3,
				age_days: 0.5,
				info_url: 'https://example.invalid/1?apikey=REDACTED',
				expires_at: '2026-08-16T12:30:00Z',
				supersedes_candidate_id: 4
			})
		).toEqual({
			candidateId: 7,
			guid: 'g',
			title: 'Some.Release.2024',
			indexerName: 'nzbgeek',
			protocol: 'usenet',
			sizeBytes: 1024,
			seeders: 3,
			ageDays: 0.5,
			infoUrl: 'https://example.invalid/1?apikey=REDACTED',
			expiresAt: '2026-08-16T12:30:00Z',
			supersedesCandidateId: 4
		});
	});

	/**
	 * ⚠️ THE FIELD IS `indexer_flags` ON THE WIRE AND `Flags` IN GO
	 * (internal/httpapi/search.go). Grepping the Go field name concludes the data
	 * is not there, which is how a whole slice shipped throwing it away. Same for
	 * `grabs`, which was on the wire and simply never read.
	 */
	it('reads grabs, leechers and indexer_flags, which the first client dropped', () => {
		const release = toRelease({
			candidate_id: 7,
			title: 'Some.Release.2024',
			indexer_id: 12,
			protocol: 'torrent',
			seeders: 41,
			leechers: 9,
			grabs: 214,
			indexer_flags: ['internal', 'freeleech', 'golden']
		});
		expect(release?.indexerId).toBe(12);
		expect(release?.grabs).toBe(214);
		expect(release?.leechers).toBe(9);
		// Open vocabulary: `golden` is one indexer's private subclass and must
		// survive. An allowlist would drop it, and drop it invisibly.
		expect(release?.indexerFlags).toEqual(['internal', 'freeleech', 'golden']);
	});

	it('leaves indexerFlags undefined when the server sends none', () => {
		// Absent means UNKNOWN, never "not freeleech" — a usenet result never
		// carries flags whatever the indexer is offering, and an empty array must
		// not become an empty chip list that reads as a claim.
		expect(toRelease({ candidate_id: 7, title: 't' })?.indexerFlags).toBeUndefined();
		expect(
			toRelease({ candidate_id: 7, title: 't', indexer_flags: [] })?.indexerFlags
		).toBeUndefined();
		expect(
			toRelease({ candidate_id: 7, title: 't', indexer_flags: 'freeleech' })?.indexerFlags
		).toBeUndefined();
	});

	it('requires a usable candidate id and a title', () => {
		expect(toRelease({ title: 'no id' })).toBeUndefined();
		expect(toRelease({ candidate_id: 1 })).toBeUndefined();
		expect(toRelease({ candidate_id: 0, title: 'zero is not an id' })).toBeUndefined();
		// The old client shape. A row with no grabbable id must not render.
		expect(toRelease({ id: 'r1', title: 'old shape' })).toBeUndefined();
	});
});

describe('toIndexerOutcome', () => {
	it('falls back to the id when the server omits the name', () => {
		expect(toIndexerOutcome({ indexer_id: 3, status: 'failed', answered: false })).toEqual({
			indexerId: 3,
			name: 'indexer 3',
			status: 'failed',
			answered: false,
			count: 0,
			reason: undefined,
			blockedUntil: undefined,
			durationMs: undefined
		});
	});

	it('rejects an entry with no indexer id', () => {
		expect(toIndexerOutcome({ name: 'nameless' })).toBeUndefined();
	});
});

describe('toReport', () => {
	it('defaults every counter rather than rendering NaN', () => {
		expect(toReport({})).toMatchObject({
			totalIndexers: 0,
			answered: 0,
			degraded: false,
			summary: '',
			indexers: []
		});
		expect(toReport('nonsense')).toBeUndefined();
	});
});

describe('problemsFrom', () => {
	it('keeps the indexers that did not answer, with the server’s own reason', () => {
		expect(
			problemsFrom([
				{ indexerId: 1, name: 'ok', status: 'answered', answered: true, count: 2 },
				{
					indexerId: 2,
					name: 'nzbgeek',
					status: 'failed',
					answered: false,
					count: 0,
					reason: 'context deadline exceeded'
				},
				{ indexerId: 3, name: 'quiet', status: 'skipped', answered: false, count: 0 }
			])
		).toEqual([
			{ indexer: 'nzbgeek', error: 'context deadline exceeded' },
			{ indexer: 'quiet', error: 'skipped' }
		]);
	});
});

describe('normalizeStreamEvent', () => {
	it('never throws on rubbish', () => {
		expect(normalizeStreamEvent('search.results', 'not json').kind).toBe('unknown');
		expect(normalizeStreamEvent('search.results', '[]').kind).toBe('unknown');
		expect(normalizeStreamEvent('whatever', '{}').kind).toBe('unknown');
	});

	it('drops a result that cannot be rendered instead of the whole frame', () => {
		const event = normalizeStreamEvent(
			'search.results',
			JSON.stringify({
				search_id: 's1',
				results: [{ title: 'no id' }, { candidate_id: 2, title: 'ok' }]
			})
		);
		expect(event).toMatchObject({ kind: 'results', searchId: 's1' });
		expect(event.kind === 'results' && event.releases.map((r) => r.candidateId)).toEqual([2]);
	});
});

describe('formatSize', () => {
	it('uses binary units', () => {
		expect(formatSize(0)).toBe('0 B');
		expect(formatSize(1024)).toBe('1.0 KiB');
		expect(formatSize(1536 * 1024 * 1024)).toBe('1.5 GiB');
		expect(formatSize(undefined)).toBe('');
	});
});

describe('formatAge', () => {
	it('turns the server’s age_days float into hours or days', () => {
		expect(formatAge(undefined)).toBe('');
		expect(formatAge(0)).toBe('1 h');
		expect(formatAge(0.5)).toBe('12 h');
		expect(formatAge(3.4)).toBe('3 d');
	});
});

describe('protocolClass', () => {
	it('only maps the two known protocols', () => {
		expect(protocolClass('torrent')).toBe('protocol-torrent');
		expect(protocolClass('usenet')).toBe('protocol-usenet');
		expect(protocolClass('carrier-pigeon')).toBe('');
	});
});

/**
 * ⚠️ `RecentGrab.id` IS AN OPAQUE ROW KEY, AND THE TWO WIRE FORMS ARE BOTH
 * PINNED HERE ON PURPOSE.
 *
 * The field used to be the numeric row id, and that leaked: the id is globally
 * monotonic across users, so somebody seeing 104 and then 341 on two of their
 * OWN grabs learns 236 rows were written by other people in between — a volume
 * oracle that survives the scope filter. The server is replacing it with a
 * keyed hash under the same field name, and is holding that change until a
 * client that treats the id as opaque is shipping.
 *
 * So this client accepts either form and normalises to a string. BOTH are
 * asserted rather than one: dropping the number case the day the hash lands
 * would silently break every client that had not updated yet, and a test that
 * pins only the NEW shape cannot catch a regression to numeric handling.
 */
describe('toRecentGrab', () => {
	const base = { release_title: 'Some.Release.2024', outcome: 'sent' };

	it('normalises today’s numeric id to a string', () => {
		expect(toRecentGrab({ ...base, id: 104 })?.id).toBe('104');
	});

	it('carries an opaque non-numeric id through unchanged, and it keys a row', () => {
		// The keyed hash. Nothing may parse it, order it, or read a length off it
		// — it is only ever compared for equality as a row key.
		const opaque = 'g_7Yb3Qx-9vKmA0zN';
		const grab = toRecentGrab({ ...base, id: opaque });
		expect(grab?.id).toBe(opaque);
		// The same value round-trips as the identity a keyed {#each} and the list
		// primitive's `data-key` compare against.
		expect(new Map([[grab!.id, grab]]).get(opaque)?.releaseTitle).toBe('Some.Release.2024');
	});

	it('still rejects a row with no usable id, and one with no title', () => {
		// Without an id there is nothing to key a row on, and without a title
		// there is nothing to recognise the grab by — which is the block's job.
		expect(toRecentGrab({ ...base })).toBeUndefined();
		expect(toRecentGrab({ ...base, id: '' })).toBeUndefined();
		expect(toRecentGrab({ ...base, id: null })).toBeUndefined();
		expect(toRecentGrab({ id: 'g_1' })).toBeUndefined();
	});

	it('does not promote a missing outcome to the confirmed wording', () => {
		// $lib/requests renders an unrecognised value as "sent, state not
		// recognised", which is true of every provenance row.
		expect(toRecentGrab({ id: 'g_1', release_title: 'x' })?.outcome).toBe('');
	});
});
