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
 * THE SERVER IS THE CONTRACT. Every event name and every payload field below is
 * the one internal/httpapi actually puts on the wire: the names are the
 * constants in internal/httpapi/events.go, the payloads are the structs in
 * internal/httpapi/search.go. The server always writes an `event:` line, so
 * there is no default `message` frame to fall back to; and it marshals Go
 * structs with fixed json tags, so there is no alternative spelling of a field
 * to guess at. Both are gone from this file on purpose — a tolerated second
 * spelling is what let the two sides disagree unnoticed in the first place.
 *
 * STREAM_EVENT_NAMES is the contract. web/src/lib/api.test.ts pins it from this
 * side and internal/httpapi/events_test.go pins it from the other, so renaming
 * an event on either side fails a test instead of silently emptying the screen.
 */

/** Every SSE event name this client understands, exactly as the server emits it. */
export const STREAM_EVENT_NAMES = [
	'search.started',
	'search.indexer',
	'search.results',
	'search.done',
	'search.failed',
	'stream.missed'
] as const;

export type StreamEventName = (typeof STREAM_EVENT_NAMES)[number];

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

/**
 * One grabbable release, as `search.results` carries it.
 *
 * candidateId is the server-side id and the only handle a grab takes; there is
 * deliberately no download or magnet url on the wire, because Prowlarr embeds
 * its full admin API key in both (internal/httpapi/search.go).
 */
export interface Release {
	candidateId: number;
	guid: string;
	title: string;
	indexerName?: string;
	protocol?: string;
	sizeBytes?: number;
	seeders?: number;
	ageDays?: number;
	infoUrl?: string;
	expiresAt?: string;
	/** The candidate this result replaces, when a higher-priority indexer answered later. */
	supersedesCandidateId?: number;
}

/** One indexer's outcome, as `search.indexer` and the `search.done` report carry it. */
export interface IndexerOutcome {
	indexerId: number;
	name: string;
	status: string;
	answered: boolean;
	count: number;
	reason?: string;
	blockedUntil?: string;
	durationMs?: number;
}

/** The closing report on `search.done`. */
export interface SearchReport {
	instanceId: number;
	query: string;
	totalIndexers: number;
	answered: number;
	failed: number;
	skipped: number;
	results: number;
	degraded: boolean;
	summary: string;
	indexers: IndexerOutcome[];
}

/** One indexer that did not answer. Shown as a banner; results keep rendering. */
export interface IndexerProblem {
	indexer: string;
	error: string;
}

/** What GET /api/v1/search answers with — 202, before any indexer has been asked. */
export interface SearchAccepted {
	searchId: string;
	instanceId?: number;
	query: string;
	type: string;
	eventsUrl?: string;
	message?: string;
}

export type StreamEvent =
	| {
			kind: 'started';
			searchId?: string;
			instanceId?: number;
			query?: string;
			searchType?: string;
	  }
	| { kind: 'indexer'; searchId?: string; phase: string; indexer: IndexerOutcome }
	| { kind: 'results'; searchId?: string; releases: Release[] }
	| { kind: 'done'; searchId?: string; report?: SearchReport }
	| { kind: 'failed'; searchId?: string; message: string; action?: string }
	| { kind: 'missed'; message: string; action?: string }
	| { kind: 'unknown' };

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function str(value: unknown): string | undefined {
	return typeof value === 'string' && value.length > 0 ? value : undefined;
}

function num(value: unknown): number | undefined {
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function bool(value: unknown): boolean {
	return value === true;
}

/**
 * One release off the wire. candidate_id and title are the two fields nothing
 * can be rendered or grabbed without, so a frame missing either is dropped
 * rather than rendered as a row with a dead Grab button.
 */
export function toRelease(value: unknown): Release | undefined {
	if (!isRecord(value)) return undefined;
	const candidateId = num(value.candidate_id);
	const title = str(value.title);
	if (candidateId === undefined || candidateId <= 0 || !title) return undefined;
	return {
		candidateId,
		guid: str(value.guid) ?? '',
		title,
		indexerName: str(value.indexer_name),
		protocol: str(value.protocol),
		sizeBytes: num(value.size_bytes),
		seeders: num(value.seeders),
		ageDays: num(value.age_days),
		infoUrl: str(value.info_url),
		expiresAt: str(value.expires_at),
		supersedesCandidateId: num(value.supersedes_candidate_id)
	};
}

export function toIndexerOutcome(value: unknown): IndexerOutcome | undefined {
	if (!isRecord(value)) return undefined;
	const indexerId = num(value.indexer_id);
	if (indexerId === undefined) return undefined;
	return {
		indexerId,
		// `name` is omitempty upstream. Falling back to the id keeps a problem
		// banner nameable rather than blank; it is not a second field spelling.
		name: str(value.name) ?? `indexer ${indexerId}`,
		status: str(value.status) ?? 'unknown',
		answered: bool(value.answered),
		count: num(value.count) ?? 0,
		reason: str(value.reason),
		blockedUntil: str(value.blocked_until),
		durationMs: num(value.duration_ms)
	};
}

export function toReport(value: unknown): SearchReport | undefined {
	if (!isRecord(value)) return undefined;
	const raw = Array.isArray(value.indexers) ? value.indexers : [];
	return {
		instanceId: num(value.instance_id) ?? 0,
		query: str(value.query) ?? '',
		totalIndexers: num(value.total_indexers) ?? 0,
		answered: num(value.answered) ?? 0,
		failed: num(value.failed) ?? 0,
		skipped: num(value.skipped) ?? 0,
		results: num(value.results) ?? 0,
		degraded: bool(value.degraded),
		summary: str(value.summary) ?? '',
		indexers: raw.map(toIndexerOutcome).filter((o): o is IndexerOutcome => o !== undefined)
	};
}

/**
 * The indexers that did not answer, as banner lines. The server's own `reason`
 * is used verbatim where it has one; `status` is the fallback so a skipped or
 * blocked indexer still says something rather than showing an empty banner.
 */
export function problemsFrom(outcomes: IndexerOutcome[]): IndexerProblem[] {
	const out: IndexerProblem[] = [];
	for (const outcome of outcomes) {
		if (outcome.answered) continue;
		out.push({ indexer: outcome.name, error: outcome.reason ?? outcome.status });
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

	const searchId = str(payload.search_id);

	switch (eventName) {
		case 'search.started':
			return {
				kind: 'started',
				searchId,
				instanceId: num(payload.instance_id),
				query: str(payload.query),
				searchType: str(payload.type)
			};
		case 'search.indexer': {
			const indexer = toIndexerOutcome(payload.indexer);
			if (!indexer) return { kind: 'unknown' };
			return { kind: 'indexer', searchId, phase: str(payload.phase) ?? '', indexer };
		}
		case 'search.results': {
			const raw = Array.isArray(payload.results) ? payload.results : [];
			const releases = raw.map(toRelease).filter((r): r is Release => r !== undefined);
			return { kind: 'results', searchId, releases };
		}
		case 'search.done':
			return { kind: 'done', searchId, report: toReport(payload.report) };
		case 'search.failed':
			return {
				kind: 'failed',
				searchId,
				message: str(payload.message) ?? 'the search stopped before it finished',
				action: str(payload.action)
			};
		case 'stream.missed':
			return {
				kind: 'missed',
				message: str(payload.message) ?? 'some events were dropped',
				action: str(payload.action)
			};
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

/**
 * Start a search. This returns as soon as the fan-out has been handed off — it
 * carries NO releases, by design (internal/httpapi/search.go): every result
 * arrives on the SSE stream as each indexer answers.
 */
export async function startSearch(query: string): Promise<SearchAccepted> {
	const url = `/api/v1/search?query=${encodeURIComponent(query)}`;
	const payload = await requestJson(url);
	if (!isRecord(payload)) {
		throw new ApiError('the backend accepted the search without a search id', 200, url);
	}
	return {
		searchId: str(payload.search_id) ?? '',
		instanceId: num(payload.instance_id),
		query: str(payload.query) ?? query,
		type: str(payload.type) ?? '',
		eventsUrl: str(payload.events_url),
		message: str(payload.message)
	};
}

export async function grabRelease(candidateId: number): Promise<void> {
	await requestJson(`/api/v1/releases/${encodeURIComponent(String(candidateId))}/grab`, {
		method: 'POST'
	});
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

	const handle = (event: MessageEvent) => onEvent(normalizeStreamEvent(event.type, event.data));
	for (const name of STREAM_EVENT_NAMES) source.addEventListener(name, handle as EventListener);
	source.addEventListener('open', () => onConnectionChange(true));
	source.addEventListener('error', () => onConnectionChange(false));

	return {
		close() {
			source.close();
			onConnectionChange(false);
		}
	};
}
