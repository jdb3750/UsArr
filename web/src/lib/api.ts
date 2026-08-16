/**
 * The UsArr HTTP surface, as this shell codes against it.
 *
 *   GET  /api/health/live              process up
 *   GET  /api/health/ready             migrations applied, listener accepting
 *   GET  /api/v1/search?query=...      starts a search; results arrive over SSE
 *   GET  /api/events                   one SSE stream, reconnects with Last-Event-ID
 *   POST /api/v1/releases/{id}/grab    grab a release candidate
 *
 * All five are implemented by internal/httpapi. The repo is still pre-alpha and
 * the rest of the surface is not, so everything below is written to degrade
 * honestly: a missing endpoint surfaces the actual status and the actual
 * upstream text, never a blank screen and never "an error occurred".
 * ARCHITECTURE.md §17.3 and §17.7 make that a product rule.
 *
 * The SSE envelope is deliberately accepted in two shapes: a named SSE event
 * (`search.result`, `search.status`, `search.done`) or a default `message`
 * event whose JSON payload carries an equivalent `type` field, tolerating
 * unknown fields. `normalizeStreamEvent` is the one place that changes.
 *
 * MISMATCH, not yet reconciled: internal/httpapi/events.go emits
 * `search.started`, `search.indexer`, `search.results`, `search.done` and
 * `search.failed`. Only `search.done` is a name this file listens for, so
 * results do not currently reach the page. Fixing it is a change to these
 * names, not to the shape.
 */

export class ApiError extends Error {
	readonly status: number;
	readonly url: string;

	constructor(message: string, status: number, url: string) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.url = url;
	}
}

export interface Release {
	id: string;
	title: string;
	indexer?: string;
	protocol?: string;
	size?: number;
	seeders?: number;
	age?: string;
	category?: string;
}

/** One indexer that did not answer. Shown as a banner; results keep rendering. */
export interface IndexerProblem {
	indexer: string;
	error: string;
}

export interface SearchStarted {
	searchId?: string;
	/** Some backends answer the first page inline; treat it as a bonus, not a contract. */
	releases: Release[];
	problems: IndexerProblem[];
	indexersTotal?: number;
	indexersDone?: number;
}

export type StreamEvent =
	| { kind: 'result'; searchId?: string; release: Release }
	| {
			kind: 'status';
			searchId?: string;
			problems: IndexerProblem[];
			indexersTotal?: number;
			indexersDone?: number;
	  }
	| { kind: 'done'; searchId?: string }
	| { kind: 'unknown' };

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null;
}

function str(value: unknown): string | undefined {
	return typeof value === 'string' && value.length > 0 ? value : undefined;
}

function num(value: unknown): number | undefined {
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

export function toRelease(value: unknown): Release | undefined {
	if (!isRecord(value)) return undefined;
	const id = str(value.id) ?? (num(value.id) !== undefined ? String(value.id) : undefined);
	const title = str(value.title);
	if (!id || !title) return undefined;
	return {
		id,
		title,
		indexer: str(value.indexer),
		protocol: str(value.protocol),
		size: num(value.size),
		seeders: num(value.seeders),
		age: str(value.age) ?? (num(value.age) !== undefined ? String(value.age) : undefined),
		category: str(value.category)
	};
}

export function toProblems(value: unknown): IndexerProblem[] {
	if (!Array.isArray(value)) return [];
	const out: IndexerProblem[] = [];
	for (const entry of value) {
		if (!isRecord(entry)) continue;
		const indexer = str(entry.indexer) ?? str(entry.name);
		const error = str(entry.error) ?? str(entry.message);
		if (indexer && error) out.push({ indexer, error });
	}
	return out;
}

/**
 * Map one raw SSE frame onto the shell's event union. Pure, so it is the part
 * that carries a unit test.
 */
export function normalizeStreamEvent(eventName: string, rawData: string): StreamEvent {
	let payload: unknown;
	try {
		payload = JSON.parse(rawData);
	} catch {
		return { kind: 'unknown' };
	}
	if (!isRecord(payload)) return { kind: 'unknown' };

	const name = eventName === 'message' ? (str(payload.type) ?? '') : eventName;
	const searchId = str(payload.searchId) ?? str(payload.search_id);

	switch (name) {
		case 'search.result': {
			const release = toRelease(payload.release ?? payload);
			return release ? { kind: 'result', searchId, release } : { kind: 'unknown' };
		}
		case 'search.status':
			return {
				kind: 'status',
				searchId,
				problems: toProblems(payload.problems ?? payload.degradedIndexers ?? payload.warnings),
				indexersTotal: num(payload.indexersTotal),
				indexersDone: num(payload.indexersDone)
			};
		case 'search.done':
			return { kind: 'done', searchId };
		default:
			return { kind: 'unknown' };
	}
}

async function readError(response: Response, url: string): Promise<ApiError> {
	// The upstream's own words, verbatim — ARCHITECTURE.md §17.3.
	let detail: string;
	try {
		detail = (await response.text()).trim();
	} catch {
		detail = '';
	}
	if (detail.length > 500) detail = `${detail.slice(0, 500)}…`;
	const message = detail ? `HTTP ${response.status}: ${detail}` : `HTTP ${response.status}`;
	return new ApiError(message, response.status, url);
}

async function requestJson(url: string, init?: RequestInit): Promise<unknown> {
	let response: Response;
	try {
		response = await fetch(url, { headers: { accept: 'application/json' }, ...init });
	} catch (cause) {
		throw new ApiError(
			`the UsArr backend could not be reached (${cause instanceof Error ? cause.message : String(cause)})`,
			0,
			url
		);
	}
	if (!response.ok) throw await readError(response, url);
	if (response.status === 204) return undefined;
	const text = await response.text();
	if (text.trim() === '') return undefined;
	try {
		return JSON.parse(text);
	} catch {
		throw new ApiError('the backend answered with a body that is not JSON', response.status, url);
	}
}

/** True when /api/health/ready answers 200. Any other outcome throws. */
export async function checkReady(): Promise<void> {
	await requestJson('/api/health/ready');
}

export async function startSearch(query: string): Promise<SearchStarted> {
	const url = `/api/v1/search?query=${encodeURIComponent(query)}`;
	const payload = await requestJson(url);
	if (!isRecord(payload)) return { releases: [], problems: [] };
	const rawReleases = Array.isArray(payload.releases) ? payload.releases : [];
	return {
		searchId: str(payload.searchId) ?? str(payload.search_id) ?? str(payload.id),
		releases: rawReleases.map(toRelease).filter((r): r is Release => r !== undefined),
		problems: toProblems(payload.problems ?? payload.degradedIndexers ?? payload.warnings),
		indexersTotal: num(payload.indexersTotal),
		indexersDone: num(payload.indexersDone)
	};
}

export async function grabRelease(id: string): Promise<void> {
	await requestJson(`/api/v1/releases/${encodeURIComponent(id)}/grab`, { method: 'POST' });
}

export interface StreamHandles {
	close(): void;
}

/**
 * Subscribe to /api/events. EventSource handles reconnection and replays
 * Last-Event-ID on its own, so there is no retry logic here on purpose.
 */
export function openEventStream(
	onEvent: (event: StreamEvent) => void,
	onConnectionChange: (connected: boolean) => void
): StreamHandles {
	const source = new EventSource('/api/events');
	const names = ['search.result', 'search.status', 'search.done'];

	const handle = (event: MessageEvent) => onEvent(normalizeStreamEvent(event.type, event.data));
	for (const name of names) source.addEventListener(name, handle as EventListener);
	source.addEventListener('message', handle as EventListener);
	source.addEventListener('open', () => onConnectionChange(true));
	source.addEventListener('error', () => onConnectionChange(false));

	return {
		close() {
			source.close();
			onConnectionChange(false);
		}
	};
}
