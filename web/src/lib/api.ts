/**
 * The UsArr HTTP surface, as this shell codes against it.
 *
 *   GET  /api/health/live              process up                    — open
 *   GET  /api/health/ready             migrations applied, listening — open
 *   GET  /api/v1/auth/session          bootstrap: CSRF token + who   — open
 *   POST /api/v1/auth/setup            create the one owner          — CSRF
 *   POST /api/v1/auth/login            start a session               — CSRF
 *   POST /api/v1/auth/logout           end it                        — CSRF + session
 *   GET  /api/v1/search?query=...      starts a search               — session
 *   GET  /api/events                   the one SSE stream            — session
 *   POST /api/v1/releases/{id}/grab    grab a release candidate      — CSRF + session
 *
 * All of them are implemented by internal/httpapi. The repo is still pre-alpha
 * and the rest of the surface is not, so everything below is written to degrade
 * honestly: a missing endpoint surfaces the actual status and the actual
 * upstream text, never a blank screen and never "an error occurred".
 * ARCHITECTURE.md §17.3 and §17.7 make that a product rule.
 *
 * THE MIDDLEWARE IS PART OF THE CONTRACT. internal/httpapi/server.go puts every
 * read behind `authenticated` and every write behind `csrfProtected` as well,
 * and `csrfProtected` (internal/httpapi/auth.go) enforces BOTH halves of the
 * double-submit design:
 *
 *   1. `Content-Type: application/json`, or 415. A cross-origin <form> cannot
 *      send that header without a preflight the browser will refuse.
 *   2. `X-CSRF-Token` equal to the non-HttpOnly `usarr_csrf` cookie, or 403.
 *
 * and every state-changing handler then calls decodeJSON, which rejects an
 * EMPTY body with 400. So a state-changing request from this client is always
 * built by postJson() below — header, token and a real JSON body together.
 * Sending any two of the three fails, which is exactly how grab used to fail.
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

/**
 * What the shell does when the server says "no session".
 *
 * Registered once by the root layout, which sends the browser to /login. It is
 * a hook rather than a direct `goto` so this module stays free of SvelteKit
 * imports and stays unit-testable in a plain node environment.
 */
type UnauthorizedHandler = () => void;

let unauthorizedHandler: UnauthorizedHandler | undefined;

export function onUnauthorized(handler: UnauthorizedHandler | undefined): void {
	unauthorizedHandler = handler;
}

function reportUnauthorized(): void {
	unauthorizedHandler?.();
}

/**
 * The CSRF token, last seen.
 *
 * The cookie is the source of truth — the server refreshes it on every
 * /api/v1/auth/session call and mints a new one on login, and it is deliberately
 * NOT HttpOnly so this client can read it (internal/httpapi/auth.go). The cached
 * copy from the last JSON response is the fallback for the one environment where
 * `document` does not exist, which is the unit tests.
 */
let cachedCsrfToken = '';

export const CSRF_COOKIE = 'usarr_csrf';
export const CSRF_HEADER = 'X-CSRF-Token';

export function csrfTokenFromCookie(): string {
	if (typeof document === 'undefined' || typeof document.cookie !== 'string') return '';
	for (const part of document.cookie.split(';')) {
		const raw = part.trim();
		const eq = raw.indexOf('=');
		if (eq < 0) continue;
		if (raw.slice(0, eq) !== CSRF_COOKIE) continue;
		return decodeURIComponent(raw.slice(eq + 1));
	}
	return '';
}

/** The token this client would echo right now, cookie first. */
export function currentCsrfToken(): string {
	return csrfTokenFromCookie() || cachedCsrfToken;
}

/** Exposed for tests and for a sign-out, which must not leave a stale token behind. */
export function forgetCsrfToken(): void {
	cachedCsrfToken = '';
}

/**
 * The token, fetching one if this client has never seen it. GET
 * /api/v1/auth/session is unauthenticated on purpose: it is the bootstrap call
 * that hands out the cookie before anyone can possibly be signed in.
 */
async function ensureCsrfToken(): Promise<string> {
	const existing = currentCsrfToken();
	if (existing) return existing;
	await fetchSession();
	const token = currentCsrfToken();
	if (!token) {
		throw new ApiError(
			'the backend did not issue a CSRF token, so nothing can be submitted',
			0,
			SESSION_URL
		);
	}
	return token;
}

async function requestJson(url: string, init?: RequestInit): Promise<unknown> {
	let response: Response;
	try {
		response = await fetch(url, {
			credentials: 'same-origin',
			...init,
			// Merged, not replaced: an init that carries Content-Type must not
			// silently drop Accept, and vice versa.
			headers: { accept: 'application/json', ...(init?.headers ?? {}) }
		});
	} catch (cause) {
		throw new ApiError(
			`the UsArr backend could not be reached (${cause instanceof Error ? cause.message : String(cause)})`,
			0,
			url
		);
	}
	if (response.status === 401) {
		// Never render a screen built from a 401. The layout sends the browser to
		// the sign-in page, which says what happened.
		reportUnauthorized();
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

/**
 * Every state-changing request goes through here, so the three things
 * csrfProtected + decodeJSON demand cannot be forgotten one at a time.
 *
 * The single retry covers the one benign 403: a token that has rotated under a
 * long-lived tab — logging in mints a new one, and a page left open across a
 * restart holds the old one. Re-bootstrapping and retrying once turns that into
 * a working click rather than an error the user cannot act on. A second 403 is
 * real and is surfaced.
 */
async function postJson(url: string, body: unknown = {}): Promise<unknown> {
	const send = async (token: string) =>
		requestJson(url, {
			method: 'POST',
			headers: { 'content-type': 'application/json', [CSRF_HEADER]: token },
			// decodeJSON rejects an empty body with 400, so there is always one.
			body: JSON.stringify(body ?? {})
		});

	try {
		return await send(await ensureCsrfToken());
	} catch (error) {
		if (!(error instanceof ApiError) || error.status !== 403) throw error;
		// Re-bootstrap unconditionally: the server reissues the cookie on this
		// call, so this is the only thing that can actually change the outcome.
		forgetCsrfToken();
		await fetchSession();
		const token = currentCsrfToken();
		if (!token) throw error;
		return send(token);
	}
}

/** True when /api/health/ready answers 200. Any other outcome throws. */
export async function checkReady(): Promise<void> {
	await requestJson('/api/health/ready');
}

// ── auth ────────────────────────────────────────────────────────────────────

export const SESSION_URL = '/api/v1/auth/session';

/**
 * GET /api/v1/auth/session, as internal/httpapi's sessionResponse spells it.
 *
 * setupRequired and authenticated are independent: a fresh install is neither,
 * and the sign-in screen has to tell those two cases apart because
 * POST /api/v1/auth/setup closes permanently — 409 already_setup — the moment
 * an owner exists.
 */
export interface SessionState {
	authenticated: boolean;
	setupRequired: boolean;
	csrfToken: string;
	userId?: number;
	username?: string;
	isOwner: boolean;
	sudoUntil?: string;
}

export const SIGNED_OUT: SessionState = {
	authenticated: false,
	setupRequired: false,
	csrfToken: '',
	isOwner: false
};

function toSessionState(payload: unknown): SessionState {
	if (!isRecord(payload)) return SIGNED_OUT;
	const state: SessionState = {
		authenticated: bool(payload.authenticated),
		setupRequired: bool(payload.setup_required),
		csrfToken: str(payload.csrf_token) ?? '',
		userId: num(payload.user_id),
		username: str(payload.username),
		isOwner: bool(payload.is_owner),
		sudoUntil: str(payload.sudo_until)
	};
	// Cache it for the one environment with no document.cookie. In a browser the
	// cookie the same response set is what actually gets echoed.
	if (state.csrfToken) cachedCsrfToken = state.csrfToken;
	return state;
}

/**
 * The bootstrap call. Unauthenticated by design, so it is also the one request
 * that must not trigger the 401 redirect — it never 401s.
 */
export async function fetchSession(): Promise<SessionState> {
	return toSessionState(await requestJson(SESSION_URL));
}

/**
 * Create the single owner account. Succeeds exactly once per install; after
 * that the server answers 409 already_setup and the caller must sign in.
 */
export async function setupOwner(username: string, password: string): Promise<SessionState> {
	return toSessionState(await postJson('/api/v1/auth/setup', { username, password }));
}

export async function login(username: string, password: string): Promise<SessionState> {
	return toSessionState(await postJson('/api/v1/auth/login', { username, password }));
}

export async function logout(): Promise<void> {
	try {
		await postJson('/api/v1/auth/logout', {});
	} finally {
		// The server cleared the session cookie; drop the token this client was
		// holding so the next sign-in bootstraps a fresh one.
		forgetCsrfToken();
	}
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

/** What POST /api/v1/releases/{id}/grab answers with (internal/httpapi/grab.go). */
export interface GrabResult {
	candidateId: number;
	releaseTitle: string;
	protocol?: string;
	indexerName?: string;
	reSearched: boolean;
	message: string;
}

/**
 * Grab a release candidate.
 *
 * The body is `{}` and NOT nothing: handleGrab calls decodeJSON, which answers
 * 400 on an empty body. grabRequest's only field is the optional `instance_id`,
 * and DisallowUnknownFields means anything else here would be a 400 too — the
 * candidate id travels in the path, and the release resource itself never
 * leaves the server (it embeds Prowlarr's admin key).
 */
export async function grabRelease(candidateId: number): Promise<GrabResult> {
	const payload = await postJson(
		`/api/v1/releases/${encodeURIComponent(String(candidateId))}/grab`,
		{}
	);
	if (!isRecord(payload)) {
		return { candidateId, releaseTitle: '', reSearched: false, message: '' };
	}
	return {
		candidateId: num(payload.candidate_id) ?? candidateId,
		releaseTitle: str(payload.release_title) ?? '',
		protocol: str(payload.protocol),
		indexerName: str(payload.indexer_name),
		reSearched: bool(payload.re_searched),
		message: str(payload.message) ?? ''
	};
}

export interface StreamHandles {
	close(): void;
}

/**
 * Subscribe to /api/events. EventSource handles reconnection and replays
 * Last-Event-ID on its own, so there is no retry logic here on purpose.
 *
 * What it does NOT do is tell us why a connection failed: the spec gives the
 * error event no status, so a 401 from `authenticated` is indistinguishable
 * from a dropped socket and the browser retries it forever. That is a silent
 * infinite loop behind a "not connected" banner — so an error triggers one
 * cheap probe of /api/v1/auth/session, and a stream that failed because the
 * session is gone stops retrying and sends the user to sign in.
 */
export function openEventStream(
	onEvent: (event: StreamEvent) => void,
	onConnectionChange: (connected: boolean) => void
): StreamHandles {
	const source = new EventSource('/api/events');
	let closed = false;
	let probing = false;

	const handle = (event: MessageEvent) => onEvent(normalizeStreamEvent(event.type, event.data));
	for (const name of STREAM_EVENT_NAMES) source.addEventListener(name, handle as EventListener);
	source.addEventListener('open', () => onConnectionChange(true));
	source.addEventListener('error', () => {
		onConnectionChange(false);
		if (closed || probing) return;
		probing = true;
		fetchSession()
			.then((state) => {
				if (closed || state.authenticated) return;
				closed = true;
				source.close();
				reportUnauthorized();
			})
			.catch(() => {
				// The backend is unreachable, which is a different problem and is
				// already visible as the disconnected banner. Let EventSource retry.
			})
			.finally(() => {
				probing = false;
			});
	});

	return {
		close() {
			closed = true;
			source.close();
			onConnectionChange(false);
		}
	};
}
