/**
 * The UsArr HTTP surface, as this shell codes against it.
 *
 *   GET  /api/health/live              process up                    — open
 *   GET  /api/health/ready             migrations applied, listening — open
 *   GET  /api/v1/auth/session          bootstrap: CSRF token + who   — open
 *   POST /api/v1/auth/setup            create the one owner          — CSRF
 *   POST /api/v1/auth/login            start a session               — CSRF
 *   POST /api/v1/auth/logout           end it                        — CSRF + session
 *   POST /api/v1/auth/sudo             reopen the 5-minute window    — CSRF + session
 *   GET  /api/v1/services              the configured instances      — session
 *   POST /api/v1/services              add one (tests it first)      — CSRF + session + sudo
 *   GET  /api/v1/services/health       per-instance state + problem  — session
 *   POST /api/v1/services/test         test an UNSAVED instance      — CSRF + session + sudo
 *   PATCH  /api/v1/services/{id}       edit one                      — CSRF + session + sudo
 *   DELETE /api/v1/services/{id}       remove one                    — CSRF + session + sudo
 *   POST /api/v1/services/{id}/test    re-test a saved one           — CSRF + session + sudo
 *   GET  /api/v1/search?query=...      starts a search               — session
 *   GET  /api/v1/indexers              the scope picker's catalogue  — session
 *   GET  /api/events                   the one SSE stream            — session
 *   POST /api/v1/releases/{id}/grab    grab a release candidate      — CSRF + session
 *   GET  /api/v1/grabs/recent          the Recent-grabs block        — session
 *
 * `sudo` is the third gate and it is NOT the same as being signed in. All FIVE
 * service writes — create, the unsaved test, PATCH, DELETE and the saved-instance
 * test (internal/httpapi/server.go) — sit behind the sudo middleware, which
 * answers 403 `sudo_required` once the five-minute window opened by signing in
 * has closed. Both connection tests are in that list because they make UsArr
 * send a stored full-admin credential upstream, so they are writes as far as the
 * threat model is concerned even though they store nothing.
 *
 * Three DIFFERENT things answer 403 here — `csrf`, `sudo_required` and
 * `forbidden` — and the code is on the body's `error` field, not a `code` field
 * (internal/httpapi/json.go errorBody). Only `csrf` is fixable by re-bootstrapping
 * the token, so sendJson retries that one and surfaces the other two; see
 * isRetryableCsrfFailure.
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

// The catalogue's shapes live in `$lib/indexercatalog`, beside the logic that
// folds them into a picker: that module is plain TypeScript and is asserted by
// `indexercatalog.test.ts`, which a `.svelte.ts` module could not be. This file
// owns only the wire parsing, which is the half that has to match Go's json
// tags.
import type {
	CatalogCategory,
	CatalogIndexer,
	CatalogInstance,
	IndexerCatalog
} from './indexercatalog';
export type {
	CatalogCategory,
	CatalogIndexer,
	CatalogInstance,
	IndexerCatalog
} from './indexercatalog';

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

	/**
	 * The `error` field of internal/httpapi's errorBody — `no_indexer_service`,
	 * `sudo_required`, `credential_reentry_required`, and so on. It is what lets
	 * a screen turn one failure into the one link that fixes it, instead of
	 * matching on prose. Empty when the body was not the server's error shape.
	 */
	readonly code: string;

	/** The server's `action`: the one thing it says fixes this. */
	readonly action: string;

	/**
	 * The server's own `message`, with no `HTTP nnn:` prefix. §17.3 wants the
	 * verbatim upstream text on screen; `message` stays prefixed because it is
	 * what a bare `String(error)` shows in a console.
	 */
	readonly detail: string;

	constructor(message: string, status: number, url: string, code = '', action = '', detail = '') {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.url = url;
		this.code = code;
		this.action = action;
		this.detail = detail || message;
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
	/** The Prowlarr-side indexer id. It is what `?indexer=` scopes on, so the
	 * picker cannot be built without it. */
	indexerId?: number;
	indexerName?: string;
	protocol?: string;
	sizeBytes?: number;
	seeders?: number;
	leechers?: number;
	/** How many times the indexer says this release has been grabbed. */
	grabs?: number;
	ageDays?: number;
	/**
	 * The indexer's own flags, VERBATIM AND UNFILTERED.
	 *
	 * ⚠️ THE WIRE NAME IS `indexer_flags` AND THE GO FIELD IS `Flags`
	 * (internal/httpapi/search.go). Grepping the Go field name concludes this
	 * data does not exist; it always did, and the client simply threw it away.
	 *
	 * `IndexerFlag` is a subclassable class in Prowlarr, not an enum — the base
	 * type declares seven and `PassThePopcornFlag` adds two more — so there is
	 * no known list to match against and there must not be one. An allowlist
	 * drops every future indexer's flags INVISIBLY, rendering fewer chips than
	 * the indexer sent with nothing saying anything was discarded.
	 *
	 * An ABSENT flag means UNKNOWN, never "not freeleech": `freeleech` and
	 * `halfleech` are derived from exact equality against a download factor of
	 * 0.0 and 0.5, so a 25% promotion produces no flag at all, and usenet
	 * results never carry flags whatever the indexer offers.
	 */
	indexerFlags?: string[];
	/** Raw Newznab category ids, the parent and the specific one together —
	 * `[2000, 2040]`. The Category column's fallback for a category the server
	 * derived no `type:` tag for. */
	categories: number[];
	/**
	 * The system tags internal/releases derived — `type:movie`, `source:torrent`,
	 * `flag:freeleech`.
	 *
	 * The Category column reads `type:` and `format:` from HERE rather than
	 * mapping the raw ids itself, and that is not laziness: `mapping.MediaType`
	 * runs TWO passes so that `[3000, 3030]` — an audiobook, which Prowlarr emits
	 * parent-first — comes out `type:book · audiobook` rather than `type:music`,
	 * which is precisely the mistake category 3030 exists to prevent. One rule,
	 * server-side.
	 */
	tags: string[];
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
 * A wire array of strings, filtered to the non-empty ones.
 *
 * Deliberately NOT validated against a known vocabulary: its only caller is
 * `indexer_flags`, which is open by construction. See Release.indexerFlags.
 */
function numArray(value: unknown): number[] {
	if (!Array.isArray(value)) return [];
	return value.filter((v): v is number => typeof v === 'number' && Number.isFinite(v));
}

function strArray(value: unknown): string[] | undefined {
	if (!Array.isArray(value)) return undefined;
	const out = value.filter((v): v is string => typeof v === 'string' && v.length > 0);
	return out.length > 0 ? out : undefined;
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
		indexerId: num(value.indexer_id),
		indexerName: str(value.indexer_name),
		protocol: str(value.protocol),
		sizeBytes: num(value.size_bytes),
		seeders: num(value.seeders),
		leechers: num(value.leechers),
		grabs: num(value.grabs),
		ageDays: num(value.age_days),
		// `indexer_flags`, not `flags`. The Go field is Flags and the JSON tag is
		// not, which is exactly how this field went unread for a whole slice.
		indexerFlags: strArray(value.indexer_flags),
		categories: numArray(value.categories),
		tags: strArray(value.tags) ?? [],
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

	// Every failure from internal/httpapi is one errorBody: {error, message,
	// action?}. Unwrapping it is what turns "HTTP 409: {"error":"no_indexer_..."
	// into a sentence plus a code a screen can branch on. A body that is not
	// that shape is kept verbatim rather than replaced with anything invented.
	let code = '';
	let action = '';
	try {
		const parsed: unknown = JSON.parse(detail);
		if (isRecord(parsed)) {
			code = str(parsed.error) ?? '';
			action = str(parsed.action) ?? '';
			detail = str(parsed.message) ?? detail;
		}
	} catch {
		// Not JSON. The raw text is still the server's own words.
	}

	if (detail.length > 500) detail = `${detail.slice(0, 500)}…`;
	const message = detail ? `HTTP ${response.status}: ${detail}` : `HTTP ${response.status}`;
	return new ApiError(message, response.status, url, code, action, detail);
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

/** The methods internal/httpapi routes through `csrfProtected`. */
type WriteMethod = 'POST' | 'PATCH' | 'DELETE';

/**
 * Every state-changing request goes through here, so the three things
 * csrfProtected + decodeJSON demand cannot be forgotten one at a time.
 *
 * PATCH and DELETE are here for the same reason POST is: `csrfProtected` wraps
 * them identically in server.go, and it checks the CONTENT TYPE before it checks
 * the token — so a DELETE issued as a bare `fetch(url, {method:'DELETE'})`
 * answers 415, not 403, and looks like a routing bug rather than a missing
 * header. DELETE carries a JSON body it does not need for exactly that reason:
 * the header is the requirement, and sending the pair together is one rule
 * instead of a per-method exception.
 *
 * The single retry covers the one benign 403: a token that has rotated under a
 * long-lived tab — logging in mints a new one, and a page left open across a
 * restart holds the old one. Re-bootstrapping and retrying once turns that into
 * a working click rather than an error the user cannot act on. A second 403 is
 * real and is surfaced.
 */
async function sendJson(
	method: WriteMethod,
	url: string,
	body: unknown = {},
	signal?: AbortSignal
): Promise<unknown> {
	const send = async (token: string) =>
		requestJson(url, {
			method,
			headers: { 'content-type': 'application/json', [CSRF_HEADER]: token },
			// decodeJSON rejects an empty body with 400, so there is always one.
			body: JSON.stringify(body ?? {}),
			// Only the connection tests pass one. §17.3 requires the `testing`
			// state to be CANCELLABLE, and a cancel that only stops listening
			// leaves a full-admin credential in flight to a host the user has just
			// decided against — so the abort is real and reaches fetch.
			signal
		});

	try {
		return await send(await ensureCsrfToken());
	} catch (error) {
		if (!(error instanceof ApiError) || error.status !== 403 || !isRetryableCsrfFailure(error)) {
			throw error;
		}
		// Re-bootstrap: the server reissues the cookie on this call, so this is
		// the only thing that can actually change the outcome.
		forgetCsrfToken();
		await fetchSession();
		const token = currentCsrfToken();
		if (!token) throw error;
		return send(token);
	}
}

/**
 * Whether a 403 is the one a fresh CSRF token can fix.
 *
 * internal/httpapi answers 403 with THREE different codes and only one of them
 * is about the token, so this is an allow-list rather than a deny-list — a
 * fourth code added server-side must default to "surface it", not to "retry it":
 *
 *   - `csrf`          — checkCSRFToken: the cookie and the header disagree, or
 *                       there is no cookie. GET /api/v1/auth/session reissues
 *                       both, so retrying once is exactly the fix.
 *   - `sudo_required` — the sudo middleware, on all five service writes. The fix
 *                       is POST /api/v1/auth/sudo with the user's password,
 *                       which only the caller can ask for. Retrying would double
 *                       every service write for nothing and still fail.
 *   - `forbidden`     — the account is disabled (authenticate), or the operation
 *                       is not permitted (handleSudo's owner check). Nothing the
 *                       client can do changes it, and a retry is one more
 *                       rejected write against a disabled account.
 *
 * A 403 whose body is not the server's error shape leaves `code` empty — a
 * proxy or a gateway in front of UsArr, not checkCSRFToken, which always names
 * itself. It is surfaced too.
 */
function isRetryableCsrfFailure(error: ApiError): boolean {
	return error.code === 'csrf';
}

async function postJson(url: string, body: unknown = {}, signal?: AbortSignal): Promise<unknown> {
	return sendJson('POST', url, body, signal);
}

/**
 * True for the ApiError a caller's own `AbortController.abort()` produced.
 *
 * requestJson wraps every fetch rejection into an ApiError with status 0, which
 * is the right shape for a screen — but an abort the USER asked for is not a
 * failure and must not be rendered as one. The DOMException's name survives in
 * the wrapped message, and the caller's own `signal.aborted` is the second
 * check, so a coincidental message cannot be mistaken for a cancellation.
 */
export function wasAborted(error: unknown, signal?: AbortSignal): boolean {
	if (signal?.aborted !== true) return false;
	return error instanceof ApiError && error.status === 0;
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
 * Reopen the five-minute sudo window (internal/httpapi/auth.go handleSudo).
 *
 * Signing in opens it, so a first run never sees this. A tab left open for six
 * minutes does, and without it every service write on that tab answers 403 and
 * there is nothing the user can click.
 */
export async function confirmSudo(password: string): Promise<SessionState> {
	return toSessionState(await postJson('/api/v1/auth/sudo', { password }));
}

// ── services ────────────────────────────────────────────────────────────────

/**
 * One configured instance, as GET /api/v1/services renders it.
 *
 * There is NO api key field here and there is not going to be one. The server
 * never returns the credential — not in full and not masked — so `hasCredential`
 * is the entire truth this client has about it, and any UI that wants to show
 * "a key is stored" shows exactly that (internal/httpapi/services.go).
 */
export interface ServiceInstance {
	id: number;
	kind: string;
	role: string;
	name: string;
	baseUrl: string;
	urlBase?: string;
	apiVersion?: string;
	enabled: boolean;
	priority: number;
	managedBy: string;
	verifyTls: boolean;
	hasCredential: boolean;
	healthState: string;
	breakerState: string;
}

/** One of the *Arr's OWN health warnings, forwarded by the background prober. */
export interface ServiceWarning {
	source?: string;
	type?: string;
	message: string;
	wikiUrl?: string;
}

/** One indexer Prowlarr is currently refusing to query. */
export interface BlockedIndexer {
	indexerId: number;
	name?: string;
	disabledTill?: string;
	mostRecentFailure?: string;
}

/**
 * One row of GET /api/v1/services/health.
 *
 * `problem` is the VERBATIM upstream error text after credential redaction, and
 * `action` is the server's own name for the one thing that fixes this state.
 * Neither is ever replaced with a generic sentence here — that is the whole
 * point of the endpoint (ARCHITECTURE.md §17.3).
 */
export interface ServiceHealth {
	id: number;
	name: string;
	kind: string;
	role: string;
	baseUrl: string;
	enabled: boolean;
	state: string;
	problem?: string;
	action?: string;
	breakerState: string;
	breakerRetryAt?: string;
	consecutiveFailures: number;
	lastOkAt?: string;
	appVersion?: string;
	warnings: ServiceWarning[];
	blockedIndexers: BlockedIndexer[];
	observedAt?: string;
	/** True when no probe has been taken yet. An honest "not measured" beats a green tick. */
	stale: boolean;
}

export interface ServicesHealth {
	services: ServiceHealth[];
	anyUnhealthy: boolean;
	/** True when nothing is configured at all: say so, do not render an empty table. */
	setupRequired: boolean;
}

/** What both connection-test endpoints answer with. */
export interface ConnectionTestResult {
	ok: boolean;
	reachable: boolean;
	/**
	 * False when the instance answered but accepts local requests without a key,
	 * so a 200 proved reachability and nothing about the credential. Say
	 * "reachable", never "verified".
	 */
	keyProvenValid: boolean;
	appName?: string;
	appVersion?: string;
	apiVersion?: string;
	message: string;
	action?: string;
}

function toServiceInstance(value: unknown): ServiceInstance | undefined {
	if (!isRecord(value)) return undefined;
	const id = num(value.id);
	if (id === undefined || id <= 0) return undefined;
	return {
		id,
		kind: str(value.kind) ?? '',
		role: str(value.role) ?? '',
		name: str(value.name) ?? '',
		baseUrl: str(value.base_url) ?? '',
		urlBase: str(value.url_base),
		apiVersion: str(value.api_version),
		enabled: bool(value.enabled),
		priority: num(value.priority) ?? 0,
		managedBy: str(value.managed_by) ?? '',
		verifyTls: bool(value.verify_tls),
		hasCredential: bool(value.has_credential),
		healthState: str(value.health_state) ?? '',
		breakerState: str(value.breaker_state) ?? ''
	};
}

function toWarning(value: unknown): ServiceWarning | undefined {
	if (!isRecord(value)) return undefined;
	const message = str(value.message);
	if (!message) return undefined;
	return {
		source: str(value.source),
		type: str(value.type),
		message,
		wikiUrl: str(value.wiki_url)
	};
}

function toBlockedIndexer(value: unknown): BlockedIndexer | undefined {
	if (!isRecord(value)) return undefined;
	const indexerId = num(value.indexer_id);
	if (indexerId === undefined) return undefined;
	return {
		indexerId,
		name: str(value.name),
		disabledTill: str(value.disabled_till),
		mostRecentFailure: str(value.most_recent_failure)
	};
}

function toServiceHealth(value: unknown): ServiceHealth | undefined {
	if (!isRecord(value)) return undefined;
	const id = num(value.id);
	if (id === undefined || id <= 0) return undefined;
	const warnings = Array.isArray(value.warnings) ? value.warnings : [];
	const blocked = Array.isArray(value.blocked_indexers) ? value.blocked_indexers : [];
	return {
		id,
		name: str(value.name) ?? '',
		kind: str(value.kind) ?? '',
		role: str(value.role) ?? '',
		baseUrl: str(value.base_url) ?? '',
		enabled: bool(value.enabled),
		state: str(value.state) ?? 'unknown',
		problem: str(value.problem),
		action: str(value.action),
		breakerState: str(value.breaker_state) ?? '',
		breakerRetryAt: str(value.breaker_retry_at),
		consecutiveFailures: num(value.consecutive_failures) ?? 0,
		lastOkAt: str(value.last_ok_at),
		appVersion: str(value.app_version),
		warnings: warnings.map(toWarning).filter((w): w is ServiceWarning => w !== undefined),
		blockedIndexers: blocked
			.map(toBlockedIndexer)
			.filter((b): b is BlockedIndexer => b !== undefined),
		observedAt: str(value.observed_at),
		stale: bool(value.stale)
	};
}

function toTestResult(value: unknown): ConnectionTestResult {
	if (!isRecord(value)) {
		return { ok: false, reachable: false, keyProvenValid: false, message: '' };
	}
	return {
		ok: bool(value.ok),
		reachable: bool(value.reachable),
		keyProvenValid: bool(value.key_proven_valid),
		appName: str(value.app_name),
		appVersion: str(value.app_version),
		apiVersion: str(value.api_version),
		message: str(value.message) ?? '',
		action: str(value.action)
	};
}

/** Every service kind v0.1 can talk to — `serviceKinds` in internal/httpapi/services.go. */
export const SERVICE_KINDS = ['prowlarr'] as const;

const DEFAULT_PORTS: Record<string, string> = { 'http:': '80', 'https:': '443' };

/**
 * A base URL reduced to `scheme://host:port`, the same shape
 * crypto.NormalizeOrigin produces server-side.
 *
 * Undefined for anything that is not an http(s) URL with a host, which is what
 * normalizeBaseURL rejects with 400 anyway.
 */
export function normalizeOrigin(raw: string): string | undefined {
	const value = raw.trim();
	if (value === '') return undefined;
	let parsed: URL;
	try {
		parsed = new URL(value);
	} catch {
		return undefined;
	}
	const scheme = parsed.protocol.toLowerCase();
	if (scheme !== 'http:' && scheme !== 'https:') return undefined;
	const host = parsed.hostname.toLowerCase().replace(/\.$/, '');
	if (host === '') return undefined;
	return `${scheme}//${host}:${parsed.port || DEFAULT_PORTS[scheme]}`;
}

/**
 * The re-entry rule from reference/security.md §1.6, applied before the request
 * rather than after the 400.
 *
 * Changing a base URL's scheme, host or port invalidates the stored API key:
 * the ciphertext is AAD-bound to the old origin and cannot be opened for the new
 * one, and sending the stored key to a host the user has just typed is the
 * exfiltration bug the rule exists to prevent. The server enforces this and
 * always will — this function exists so the FORM can say "changing the address
 * means re-entering the key" while the user is still typing, instead of letting
 * them press Save and receive a 400 whose cause is invisible.
 *
 * It fails closed: a URL neither side can parse counts as changed.
 */
export function requiresCredentialReentry(current: string, next: string): boolean {
	if (next.trim() === '') return false; // not editing the address at all
	const before = normalizeOrigin(current);
	const after = normalizeOrigin(next);
	if (before === undefined || after === undefined) return true;
	return before !== after;
}

/** The message the re-entry rule produces, in the server's own words. */
const REENTRY_MESSAGE =
	'changing this service’s scheme, host or port invalidates the stored API key: ' +
	'the key is bound to the old host and cannot be moved';

function reentryError(url: string): ApiError {
	return new ApiError(
		`HTTP 400: ${REENTRY_MESSAGE}`,
		400,
		url,
		'credential_reentry_required',
		'Re-enter the API key',
		REENTRY_MESSAGE
	);
}

export const SERVICES_URL = '/api/v1/services';

/** GET /api/v1/services. Never carries a credential; see ServiceInstance. */
export async function listServices(): Promise<ServiceInstance[]> {
	const payload = await requestJson(SERVICES_URL);
	if (!isRecord(payload) || !Array.isArray(payload.services)) return [];
	return payload.services
		.map(toServiceInstance)
		.filter((s): s is ServiceInstance => s !== undefined);
}

/** GET /api/v1/services/health. Renders entirely from SQLite plus the last probe. */
export async function fetchServicesHealth(): Promise<ServicesHealth> {
	const payload = await requestJson(`${SERVICES_URL}/health`);
	if (!isRecord(payload)) {
		return { services: [], anyUnhealthy: false, setupRequired: true };
	}
	const raw = Array.isArray(payload.services) ? payload.services : [];
	return {
		services: raw.map(toServiceHealth).filter((s): s is ServiceHealth => s !== undefined),
		anyUnhealthy: bool(payload.any_unhealthy),
		setupRequired: bool(payload.setup_required)
	};
}

export interface NewService {
	kind: string;
	name: string;
	baseUrl: string;
	apiKey: string;
	/**
	 * The sub-path a reverse proxy serves this instance under — Prowlarr's own
	 * `URL Base`, e.g. `/prowlarr` for an instance reachable at
	 * `https://host/prowlarr`. It is a SEPARATE column from base_url on purpose:
	 * crypto.NormalizeHostPort discards the path, so the stored credential is
	 * bound to scheme://host:port and moving a service to a different sub-path
	 * does not move the credential (internal/crypto/derive.go).
	 *
	 * Sent only when non-empty, and as typed. The server owns the rule — it
	 * repairs a missing leading slash, drops a trailing one, and refuses anything
	 * that is not a path with `invalid_url_base` (normalizeServiceURLBase in
	 * internal/httpapi/services.go). Normalising it here too would be a second
	 * implementation that could quietly disagree with the first, and the value
	 * that reaches SQLite is the one the server decided on.
	 */
	urlBase?: string;
}

/**
 * The sub-path field's inline problem, or nothing when this failure is about
 * something else.
 *
 * A screen calls this rather than matching on prose: `invalid_url_base` is the
 * server's code for "url_base is not a path", and it is the difference between
 * putting the message under the field the user got wrong and dropping it in a
 * banner at the top of the form.
 */
export function urlBaseProblem(error: unknown): string | undefined {
	return error instanceof ApiError && error.code === 'invalid_url_base' ? error.detail : undefined;
}

/** `url_base` for a request body, or nothing at all when there is no sub-path. */
function urlBaseField(urlBase: string | undefined): { url_base?: string } {
	const value = urlBase?.trim() ?? '';
	return value === '' ? {} : { url_base: value };
}

/**
 * POST /api/v1/services/test — the wizard's test for an instance that has no row
 * yet. Optional: handleCreateService runs the same test itself and REFUSES to
 * save a service that fails it, so this endpoint is the "check before I commit"
 * affordance, not the thing that makes the save safe.
 */
export async function testNewService(
	input: NewService,
	signal?: AbortSignal
): Promise<ConnectionTestResult> {
	return toTestResult(
		await postJson(
			`${SERVICES_URL}/test`,
			{
				kind: input.kind,
				base_url: input.baseUrl.trim(),
				...urlBaseField(input.urlBase),
				api_key: input.apiKey
			},
			signal
		)
	);
}

/**
 * POST /api/v1/services. The body carries ONLY the fields createServiceRequest
 * declares: decodeJSON runs with DisallowUnknownFields, so one extra key is a
 * 400 rather than an ignored setting.
 */
export async function createService(input: NewService): Promise<ServiceInstance> {
	const created = toServiceInstance(
		await postJson(SERVICES_URL, {
			kind: input.kind,
			name: input.name.trim(),
			base_url: input.baseUrl.trim(),
			...urlBaseField(input.urlBase),
			api_key: input.apiKey
		})
	);
	if (!created) {
		throw new ApiError('the service was saved but the response had no id', 200, SERVICES_URL);
	}
	return created;
}

export interface ServiceEdit {
	name?: string;
	baseUrl?: string;
	/**
	 * The reverse-proxy sub-path, as NewService.urlBase describes it.
	 *
	 * On PATCH this field is present-means-set: supplying `''` CLEARS the stored
	 * sub-path, which is the only way to undo one, so it is sent whenever the
	 * caller passes it and omitted only when the caller does not. That differs
	 * from create and from the connection tests, where an empty value carries no
	 * information and is left off the wire entirely.
	 */
	urlBase?: string;
	apiKey?: string;
	enabled?: boolean;
	/**
	 * The address currently stored for this instance. Supplying it lets the
	 * re-entry rule be applied here, before a request that would 400.
	 */
	currentBaseUrl?: string;
}

/** PATCH /api/v1/services/{id}. */
export async function updateService(id: number, edit: ServiceEdit): Promise<ServiceInstance> {
	const url = `${SERVICES_URL}/${encodeURIComponent(String(id))}`;
	const key = edit.apiKey?.trim() ?? '';
	if (
		edit.baseUrl !== undefined &&
		edit.currentBaseUrl !== undefined &&
		key === '' &&
		requiresCredentialReentry(edit.currentBaseUrl, edit.baseUrl)
	) {
		throw reentryError(url);
	}

	const body: Record<string, unknown> = {};
	if (edit.name !== undefined) body.name = edit.name.trim();
	if (edit.baseUrl !== undefined) body.base_url = edit.baseUrl.trim();
	// No re-entry check for this one, deliberately. handleUpdateService derives
	// hostChanged from crypto.NormalizeOrigin(base_url) alone, and NormalizeOrigin
	// discards the path — so a sub-path edit on the same scheme, host and port
	// leaves the AAD, and therefore the stored credential, untouched. Demanding
	// the key back for it would be a rule this codebase does not have.
	if (edit.urlBase !== undefined) body.url_base = edit.urlBase.trim();
	if (edit.enabled !== undefined) body.enabled = edit.enabled;
	if (key !== '') body.api_key = edit.apiKey;

	const updated = toServiceInstance(await sendJson('PATCH', url, body));
	if (!updated) throw new ApiError('the service was saved but did not come back', 200, url);
	return updated;
}

/**
 * POST /api/v1/services/{id}/test.
 *
 * With no overrides this re-tests the saved instance using the stored
 * credential. With a changed base URL the stored key is neither used nor
 * usable, so a key must be supplied — same rule, same reason as the edit above.
 */
export async function testService(
	id: number,
	edit: Pick<ServiceEdit, 'baseUrl' | 'urlBase' | 'apiKey' | 'currentBaseUrl'> = {},
	signal?: AbortSignal
): Promise<ConnectionTestResult> {
	const url = `${SERVICES_URL}/${encodeURIComponent(String(id))}/test`;
	const key = edit.apiKey?.trim() ?? '';
	if (
		edit.baseUrl !== undefined &&
		edit.currentBaseUrl !== undefined &&
		key === '' &&
		requiresCredentialReentry(edit.currentBaseUrl, edit.baseUrl)
	) {
		throw reentryError(url);
	}

	const body: Record<string, unknown> = {};
	if (edit.baseUrl !== undefined && edit.baseUrl.trim() !== '') body.base_url = edit.baseUrl.trim();
	// Omitted when empty, and not as a style choice: handleTestService reads this
	// through stringOr(), where an empty string means "use the stored sub-path".
	// Sending "" could not clear it and would only look as if it might.
	Object.assign(body, urlBaseField(edit.urlBase));
	if (key !== '') body.api_key = edit.apiKey;
	return toTestResult(await postJson(url, body, signal));
}

/**
 * DELETE /api/v1/services/{id}. A soft delete server-side: the id stays burned
 * so a stale reference resolves to "gone" rather than to some other service.
 */
export async function deleteService(id: number): Promise<void> {
	await sendJson('DELETE', `${SERVICES_URL}/${encodeURIComponent(String(id))}`, {});
}

/**
 * Start a search. This returns as soon as the fan-out has been handed off — it
 * carries NO releases, by design (internal/httpapi/search.go): every result
 * arrives on the SSE stream as each indexer answers.
 */
/**
 * How wide a search is allowed to go.
 *
 * The server narrows the fan-out BEFORE the legs are planned
 * (internal/httpapi/search.go builds releases.Query from these and hands it to
 * the searcher), so an unselected indexer is never asked and spends none of its
 * rate limit. That is the whole reason scoping belongs on the request rather
 * than in a client-side filter over the results.
 *
 * An EMPTY or absent `indexerIds` means every indexer, which is the server's own
 * default. It is not the same as a one-element list, and the difference is
 * visible to the user, so the caller must not collapse them.
 */
export interface SearchScope {
	/**
	 * The `type=` value internal/httpapi/search.go accepts — `search`, `movie`,
	 * `tvsearch`, `music`, `book`. Omitted rather than sent empty: the server's
	 * setSearchType maps "" to a basic search, so both spellings mean the same
	 * thing and the shorter URL is the one that reads in a log. An unrecognised
	 * value is a 400 on purpose — Prowlarr itself would silently degrade to a
	 * basic search, which looks like the filter did nothing.
	 *
	 * It lives in this object rather than in a second positional parameter. It
	 * WAS a positional, and the comment there recorded that as a merge decision
	 * rather than a design one: two screens called this at the time, and folding
	 * it would have meant editing one of them from the other's thread. There is
	 * one caller now — the release-search surface is on Requests and nowhere else
	 * — so the shape is tidied. A scalar positional beside an object positional is
	 * exactly the call site people get wrong later.
	 */
	type?: string;
	/** Prowlarr indexer ids. Repeated as `?indexer=` — the server reads them
	 * with queryInt32s, which takes the parameter more than once. */
	indexerIds?: number[];
	/** Newznab category ids, repeated as `?category=`. */
	categories?: number[];
	/** Which configured indexer service to search, when more than one is enabled. */
	instanceId?: number;
}

/** Only finite non-negative integers reach the wire: the server answers 400 on
 * anything else, and a NaN in the URL would fail the whole search rather than
 * one filter. */
function appendIds(params: URLSearchParams, name: string, ids: number[] | undefined): void {
	for (const id of ids ?? []) {
		if (Number.isSafeInteger(id) && id >= 0) params.append(name, String(id));
	}
}

export async function startSearch(query: string, scope: SearchScope = {}): Promise<SearchAccepted> {
	const params = new URLSearchParams({ query });
	if (scope.type) params.set('type', scope.type);
	appendIds(params, 'indexer', scope.indexerIds);
	appendIds(params, 'category', scope.categories);
	if (scope.instanceId !== undefined && scope.instanceId > 0) {
		params.set('instance', String(scope.instanceId));
	}
	const url = `/api/v1/search?${params.toString()}`;
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

/**
 * One row of the Recent-grabs block (internal/httpapi/grabs.go).
 *
 * `outcome` is the field to render, NOT `acquisitionState`. The server derives
 * it precisely so a client does not have to know that "unconfirmed" means
 * "sent, and Prowlarr reported a problem afterwards" rather than "it did not
 * happen"; `acquisitionState` travels beside it as the column verbatim, because
 * the store's vocabulary is open by design and flattening it on the way through
 * is how a value the schema deliberately does not police gets lost.
 *
 * ⚠️ THE READ IS A UNION OF TWO TABLES AND THE ARMS HAVE DIFFERENT SHAPES.
 * `provenance` carries every grab that was handed over; `audit_log` carries the
 * ones that provably never left the process, and those have no provenance row —
 * so `categories`, `sizeBytes`, `downloadId`, `sourceSystem` and
 * `acquisitionState` are ABSENT on them rather than empty, and `errorCode` is
 * present only there. The absences are the point: `sizeBytes` defaulted to 0
 * would render "0 B", which is a claim about a file that does not exist.
 *
 * There is no download URL and no info URL, by construction on the server side:
 * a Prowlarr proxy link carries its full admin API key, and an indexer URL is
 * where a private tracker's passkey lives. There is no error MESSAGE either, and
 * that is the same rule: the sentence is assembled from an upstream error string
 * and shipping it would mean loosening this endpoint's response guard to carry
 * it. The code carries the identity, and §17.5's action rule branches on the
 * identity.
 */
export interface RecentGrab {
	/**
	 * ⚠️ AN OPAQUE ROW KEY. A STRING, AND NOTHING MAY READ MEANING INTO IT.
	 *
	 * It used to be the numeric row id, and that leaked: the id is globally
	 * monotonic across users, so somebody seeing 104 and then 341 on two of their
	 * OWN grabs learns that 236 rows were written by other people in between. A
	 * volume oracle that SURVIVES THE SCOPE FILTER, because the filter decides
	 * which rows come back and not what their ids reveal about the ones that did
	 * not. The server is replacing it with a keyed hash under the same field name.
	 *
	 * So it keys a row and it does nothing else. Never sort on it, never compare
	 * it numerically, never do arithmetic with it, never infer anything from its
	 * shape or its length — an id that looks numeric today is not a promise. The
	 * rows arrive newest first and that ordering is the SERVER'S; this field is
	 * not a proxy for it.
	 */
	id: string;
	/**
	 * ⚠️ ALWAYS PRESENT, AND MAY LEGITIMATELY BE `""`.
	 *
	 * On the not-sent arm the name comes from the audit row's metadata, and the
	 * candidate-not-found path has none: `release_candidate` is swept 25 minutes
	 * after the search, so by the time that row was written the only place the
	 * title ever lived no longer had it. The empty string is therefore a FACT —
	 * "the listing was already gone" — and $lib/requests renders it as one rather
	 * than as a blank cell or an invented name.
	 */
	releaseTitle: string;
	protocol?: string;
	indexerName?: string;
	/** Absent on the not-sent arm, which has no provenance row to read them from.
	 * The block renders no category column today, so this is carried rather than
	 * used — see the two-pass reason in $lib/requests.categoryLabel. */
	categories: number[];
	/** Absent rather than zero where there is no provenance row. Never default
	 * it: 0 renders as "0 B", which is a claim about a file that does not exist. */
	sizeBytes?: number;
	/** RFC 3339. Absent where the stored value would not parse — the server
	 * drops it rather than guessing, because a wrong timestamp on an
	 * irreversible action is worse than a missing one.
	 *
	 * It is the time of the ATTEMPT on both arms, and the union's sort key: on a
	 * provenance row it is written at dispatch, and on a not-sent row it is when
	 * the user pressed Grab. So the column it feeds is labelled Time, never
	 * "Downloaded" — UsArr stops observing at handoff and never learns when a
	 * download finished. */
	grabbedAt?: string;
	/** nzo_id, or the infohash for a torrent: the string that finds this grab
	 * in the download client, which is where the truth about it lives. */
	downloadId?: string;
	sourceSystem?: string;
	/**
	 * ⚠️ ITS ABSENCE IS INFORMATION, WHICH IS WHY IT IS OPTIONAL RATHER THAN
	 * DEFAULTED TO `''`. A not-sent row has no provenance row and therefore no
	 * acquisition state at all; `''` would read as a state whose name nobody
	 * knows, which is a different and wrong claim. The server omits the key for
	 * exactly that reason, and flattening it here would throw the distinction
	 * away one layer below where it was made.
	 *
	 * Nothing RENDERS off it — `outcome` is the derived field the server publishes
	 * for that — so this is the column verbatim and a cross-check, no more.
	 */
	acquisitionState?: string;
	/**
	 * The apiError code of a grab that never left, from errorcodes.go's closed
	 * vocabulary. Present only on the not-sent arm; a handed-over grab has no
	 * error to name. §17.5's action rule branches on THIS and never on prose,
	 * because there is no prose on this surface and there will not be.
	 */
	errorCode?: string;
	/** `sent` | `sent_outcome_unknown` | `not_sent` | `unknown`. See $lib/requests
	 * — and match the full string: `not_sent` deliberately does not begin with
	 * one of the handed-over values. */
	outcome: string;
}

/**
 * The row key off the wire, normalised to a string.
 *
 * BOTH WIRE FORMS ARE ACCEPTED, and that is a migration order rather than
 * tolerance for a second spelling. Today's server sends a number; the change
 * that removes the volume oracle sends an opaque string, and it is being held
 * until a client that treats the field as opaque is already shipping. Accepting
 * both is what lets the two land in either order without a broken screen in
 * between. Neither form is read for anything but identity — see RecentGrab.id.
 *
 * A row with no usable id is still rejected: there is nothing to key it on.
 */
function rowKey(value: unknown): string | undefined {
	if (typeof value === 'string') return value.length > 0 ? value : undefined;
	if (typeof value === 'number' && Number.isFinite(value)) return String(value);
	return undefined;
}

export function toRecentGrab(value: unknown): RecentGrab | undefined {
	if (!isRecord(value)) return undefined;
	const id = rowKey(value.id);
	// ⚠️ THE ID IS THE ONLY REQUIRED FIELD, AND A MISSING TITLE NO LONGER DROPS
	// THE ROW. It used to, on the reasoning that a grab you cannot recognise is
	// not worth a line — true while every row came from `provenance`, whose
	// release_title is always written. The not-sent arm broke it: the
	// candidate-not-found path has no title BECAUSE the listing was swept, which
	// is precisely the row a user is looking for when they ask why pressing Grab
	// did nothing. Dropping it would have hidden that grab entirely, and the
	// block would have been silently short by the rows it exists to explain.
	if (id === undefined) return undefined;
	const rawCategories = Array.isArray(value.categories) ? value.categories : [];
	return {
		id,
		// `?? ''` rather than a guard: the empty string is a value on this wire and
		// $lib/requests has a rendering for it. See RecentGrab.releaseTitle.
		releaseTitle: str(value.release_title) ?? '',
		protocol: str(value.protocol),
		indexerName: str(value.indexer_name),
		categories: rawCategories.map(num).filter((n): n is number => n !== undefined),
		// num() answers undefined for a non-number, so an absent size stays absent
		// and never becomes 0 — which would render as "0 B".
		sizeBytes: num(value.size_bytes),
		grabbedAt: str(value.grabbed_at),
		downloadId: str(value.download_id),
		sourceSystem: str(value.source_system),
		// Left undefined where the key is absent, because the absence is the signal
		// that there is no provenance row behind this line. See the field's note.
		acquisitionState: str(value.acquisition_state),
		errorCode: str(value.error_code),
		// An absent outcome is NOT defaulted to `sent`, and just as deliberately is
		// not defaulted to `not_sent`: $lib/requests renders an unrecognised value
		// as "sent, state not recognised", which is true of every provenance row.
		// Promoting it to the confirmed wording would be the overclaim §17.5 bans;
		// promoting it to the not-sent wording would assert the one thing this
		// screen may only say when the server has said it first.
		outcome: str(value.outcome) ?? ''
	};
}

export interface RecentGrabs {
	grabs: RecentGrab[];
	/** The limit the server actually applied — it clamps, so a client that
	 * asked for more must not read the short answer as "that is all there is". */
	limit: number;
}

export const RECENT_GRABS_URL = '/api/v1/grabs/recent';

/** The ten most recent acquisitions, newest first. A local SQLite read: it
 * makes no upstream call and is never on a path that waits for Prowlarr. */
export async function fetchRecentGrabs(limit = 10): Promise<RecentGrabs> {
	const payload = await requestJson(`${RECENT_GRABS_URL}?limit=${encodeURIComponent(limit)}`);
	if (!isRecord(payload)) return { grabs: [], limit };
	const raw = Array.isArray(payload.grabs) ? payload.grabs : [];
	return {
		grabs: raw.map(toRecentGrab).filter((g): g is RecentGrab => g !== undefined),
		limit: num(payload.limit) ?? limit
	};
}

/* ── the indexer catalogue ────────────────────────────────────────────────── */

export const INDEXERS_URL = '/api/v1/indexers';

/**
 * One category off the wire, recursively.
 *
 * ⚠️ THE RAW NEWZNAB TREE, NEVER COLLAPSED. `sub_categories` is the whole reason
 * category scoping is possible at all: 3030 sits under Audio (3000) and is the
 * only machine-readable signal separating an audiobook from music, and 7030
 * likewise for comics (ARCHITECTURE.md §8.5). Anything that folded a child into
 * its parent here would throw that away before the picker ever saw it.
 *
 * Depth is bounded by recursion on a well-formed tree; a malformed one simply
 * yields fewer levels rather than throwing, because a bad category must not be
 * able to take a whole picker down.
 */
function toCatalogCategory(value: unknown): CatalogCategory | undefined {
	if (!isRecord(value)) return undefined;
	const id = num(value.id);
	if (id === undefined) return undefined;
	const raw = Array.isArray(value.sub_categories) ? value.sub_categories : [];
	return {
		id,
		name: str(value.name) ?? '',
		children: raw.map(toCatalogCategory).filter((c): c is CatalogCategory => c !== undefined)
	};
}

/** One indexer off the wire. `indexer_id` is the value `?indexer=` sends back to
 * the search endpoint, so a row without one cannot be picked and is dropped. */
export function toCatalogIndexer(value: unknown): CatalogIndexer | undefined {
	if (!isRecord(value)) return undefined;
	const indexerId = num(value.indexer_id);
	if (indexerId === undefined) return undefined;
	const cats = Array.isArray(value.categories) ? value.categories : [];
	return {
		instanceId: num(value.instance_id) ?? 0,
		instanceName: str(value.instance_name) ?? '',
		indexerId,
		name: str(value.name) ?? `indexer ${indexerId}`,
		protocol: str(value.protocol) ?? '',
		privacy: str(value.privacy) ?? '',
		// ⚠️ NOT `?? true`. The upstream endpoint returns enabled AND disabled
		// indexers with no filter parameter, so `false` is an ordinary value here
		// and defaulting a missing one to true would offer a choice the search
		// planner then silently refuses.
		enabled: bool(value.enabled),
		priority: num(value.priority) ?? 0,
		supportsSearch: bool(value.supports_search),
		searchTypes: strArray(value.search_types) ?? [],
		categories: cats.map(toCatalogCategory).filter((c): c is CatalogCategory => c !== undefined),
		fetchedAt: str(value.fetched_at) ?? ''
	};
}

function toCatalogInstance(value: unknown): CatalogInstance | undefined {
	if (!isRecord(value)) return undefined;
	const instanceId = num(value.instance_id);
	if (instanceId === undefined) return undefined;
	return {
		instanceId,
		name: str(value.name) ?? '',
		kind: str(value.kind) ?? '',
		enabled: bool(value.enabled),
		status: str(value.status) ?? '',
		message: str(value.message) ?? '',
		action: str(value.action) ?? '',
		// Null per instance, and the null is load-bearing: it is what separates
		// "UsArr has never managed to read this list" from "it answered none".
		fetchedAt: str(value.fetched_at),
		indexerCount: num(value.indexer_count) ?? 0
	};
}

/**
 * The configured indexers, for the Requests screen's scope picker.
 *
 * ⚠️ A TIER 0 LOCAL READ. `internal/httpapi/indexers.go` reads the
 * `indexer_catalog` replica the background prober writes and makes NO upstream
 * call — that is the only reason it may sit under a render path at all
 * (ARCHITECTURE.md §2.3 rule 1). Callers must not draw a spinner or a skeleton
 * over it, and must not block a paint on it.
 *
 * ⚠️ ALL FOUR STATES ARRIVE ON A 200. `not_configured`, `never_fetched`, `empty`
 * and `service_disabled` are states of an install, not failures, so the branch
 * is on `status` and never on the HTTP code — and `indexers` is `[]` rather than
 * null in every one of them. Only a malformed request fails, which this one
 * cannot be: it sends no parameters.
 */
export async function fetchIndexerCatalog(): Promise<IndexerCatalog> {
	const payload = await requestJson(INDEXERS_URL);
	if (!isRecord(payload)) {
		return { status: '', message: '', action: '', instances: [], indexers: [] };
	}
	const instances = Array.isArray(payload.instances) ? payload.instances : [];
	const indexers = Array.isArray(payload.indexers) ? payload.indexers : [];
	return {
		status: str(payload.status) ?? '',
		message: str(payload.message) ?? '',
		action: str(payload.action) ?? '',
		instances: instances
			.map(toCatalogInstance)
			.filter((i): i is CatalogInstance => i !== undefined),
		indexers: indexers.map(toCatalogIndexer).filter((i): i is CatalogIndexer => i !== undefined)
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
