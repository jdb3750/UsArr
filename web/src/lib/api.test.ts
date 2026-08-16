import { describe, expect, it } from 'vitest';
import { normalizeStreamEvent, toProblems, toRelease } from './api';
import { formatSize, protocolClass } from './format';

describe('normalizeStreamEvent', () => {
	it('reads a named search.result frame', () => {
		const event = normalizeStreamEvent(
			'search.result',
			JSON.stringify({
				searchId: 's1',
				release: { id: 'r1', title: 'Some.Release.2024', protocol: 'usenet', size: 1024 }
			})
		);
		expect(event).toEqual({
			kind: 'result',
			searchId: 's1',
			release: {
				id: 'r1',
				title: 'Some.Release.2024',
				indexer: undefined,
				protocol: 'usenet',
				size: 1024,
				seeders: undefined,
				age: undefined,
				category: undefined
			}
		});
	});

	it('reads the same thing off a default message frame carrying a type field', () => {
		const event = normalizeStreamEvent(
			'message',
			JSON.stringify({ type: 'search.result', release: { id: 'r2', title: 'Another' } })
		);
		expect(event.kind).toBe('result');
	});

	it('collects the degraded-indexer signal off a status frame', () => {
		const event = normalizeStreamEvent(
			'search.status',
			JSON.stringify({
				searchId: 's1',
				indexersTotal: 4,
				indexersDone: 3,
				problems: [{ indexer: 'nzbgeek', error: 'context deadline exceeded' }]
			})
		);
		expect(event).toMatchObject({
			kind: 'status',
			indexersTotal: 4,
			indexersDone: 3,
			problems: [{ indexer: 'nzbgeek', error: 'context deadline exceeded' }]
		});
	});

	it('never throws on rubbish', () => {
		expect(normalizeStreamEvent('search.result', 'not json').kind).toBe('unknown');
		expect(normalizeStreamEvent('search.result', '[]').kind).toBe('unknown');
		expect(normalizeStreamEvent('whatever', '{}').kind).toBe('unknown');
	});
});

describe('toRelease', () => {
	it('requires an id and a title', () => {
		expect(toRelease({ title: 'no id' })).toBeUndefined();
		expect(toRelease({ id: 'x' })).toBeUndefined();
		expect(toRelease({ id: 7, title: 'numeric id' })?.id).toBe('7');
	});
});

describe('toProblems', () => {
	it('accepts either field naming and drops incomplete entries', () => {
		expect(toProblems([{ name: 'a', message: 'down' }, { indexer: 'b' }])).toEqual([
			{ indexer: 'a', error: 'down' }
		]);
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

describe('protocolClass', () => {
	it('only maps the two known protocols', () => {
		expect(protocolClass('torrent')).toBe('protocol-torrent');
		expect(protocolClass('usenet')).toBe('protocol-usenet');
		expect(protocolClass('carrier-pigeon')).toBe('');
	});
});
