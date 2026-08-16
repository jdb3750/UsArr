import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	ApiError,
	CSRF_HEADER,
	currentCsrfToken,
	fetchSession,
	forgetCsrfToken,
	grabRelease,
	login,
	logout,
	onUnauthorized,
	setupOwner
} from './api';

/**
 * The client half of the auth contract.
 *
 * internal/httpapi puts every read behind `authenticated` and every write behind
 * `csrfProtected` as well. `csrfProtected` answers 415 without
 * `Content-Type: application/json` and 403 without an `X-CSRF-Token` matching
 * the `usarr_csrf` cookie, and the handlers then call decodeJSON, which answers
 * 400 on an empty body. The old client sent NONE of the three on grab, so the
 * button could not work — and no test noticed, because every test drove the API
 * directly and built its own headers.
 *
 * These tests inspect what the client actually puts on the wire.
 * cmd/usarr/browserflow_test.go asserts the same shapes against the real server.
 */

type Call = { url: string; init: RequestInit };

const SESSION_TOKEN = 'csrf-token-from-the-server';

let calls: Call[];

/** Records every request and answers from a per-URL script. */
function stubFetch(script: (url: string, init: RequestInit, n: number) => Response) {
	calls = [];
	vi.stubGlobal('fetch', (input: string, init: RequestInit = {}) => {
		const url = String(input);
		calls.push({ url, init });
		return Promise.resolve(script(url, init, calls.length));
	});
}

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'content-type': 'application/json' }
	});
}

const sessionBody = {
	authenticated: true,
	setup_required: false,
	csrf_token: SESSION_TOKEN,
	user_id: 1,
	username: 'joe',
	is_owner: true
};

/** The one place a request's headers are read, so the tests agree on the shape. */
function headersOf(init: RequestInit): Record<string, string> {
	const out: Record<string, string> = {};
	for (const [key, value] of Object.entries((init.headers ?? {}) as Record<string, string>)) {
		out[key.toLowerCase()] = value;
	}
	return out;
}

beforeEach(() => {
	forgetCsrfToken();
	onUnauthorized(undefined);
});

afterEach(() => {
	vi.unstubAllGlobals();
	onUnauthorized(undefined);
});

describe('CSRF token acquisition', () => {
	it('bootstraps from GET /api/v1/auth/session before the first state-changing call', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse({ candidate_id: 7, release_title: 'x', message: 'sent' })
		);

		expect(currentCsrfToken()).toBe('');
		await grabRelease(7);

		expect(calls.map((c) => c.url)).toEqual(['/api/v1/auth/session', '/api/v1/releases/7/grab']);
		expect(currentCsrfToken()).toBe(SESSION_TOKEN);
	});

	it('reuses the token it already holds', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session' ? jsonResponse(sessionBody) : jsonResponse({})
		);

		await fetchSession();
		await grabRelease(1);
		await grabRelease(2);

		expect(calls.filter((c) => c.url === '/api/v1/auth/session')).toHaveLength(1);
	});
});

/**
 * The grab request shape, asserted field by field against what
 * `csrfProtected` + `decodeJSON` accept. This is the assertion that fails on the
 * old client: it sent `{ method: 'POST' }` and nothing else.
 */
describe('the grab request the client builds', () => {
	it('satisfies csrfProtected and decodeJSON', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse({
						candidate_id: 42,
						release_title: 'Test.Release',
						protocol: 'torrent',
						indexer_name: 'Working Tracker',
						re_searched: false,
						message: "sent to Prowlarr's download client"
					})
		);

		const result = await grabRelease(42);

		const grab = calls.find((c) => c.url === '/api/v1/releases/42/grab');
		expect(grab, 'the grab must be issued').toBeDefined();
		const headers = headersOf(grab!.init);

		// 1. csrfProtected → checkContentTypeJSON, else 415.
		expect(grab!.init.method).toBe('POST');
		expect(headers['content-type']).toBe('application/json');

		// 2. csrfProtected → checkCSRFToken, else 403. The header must carry the
		//    same value the usarr_csrf cookie holds; here that is the token the
		//    bootstrap response handed over.
		expect(headers[CSRF_HEADER.toLowerCase()]).toBe(SESSION_TOKEN);

		// 3. decodeJSON, else 400 on an empty body. And DisallowUnknownFields is
		//    on, so `instance_id` is the ONLY key grabRequest would tolerate.
		expect(typeof grab!.init.body).toBe('string');
		const body = JSON.parse(grab!.init.body as string);
		expect(body).toEqual({});
		expect(Object.keys(body).every((key) => key === 'instance_id')).toBe(true);

		// The cookie has to travel too, or `authenticated` answers 401.
		expect(grab!.init.credentials).toBe('same-origin');

		expect(result).toMatchObject({
			candidateId: 42,
			releaseTitle: 'Test.Release',
			message: "sent to Prowlarr's download client"
		});
	});

	it('sends a body, never nothing — decodeJSON rejects an empty one with 400', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session' ? jsonResponse(sessionBody) : jsonResponse({})
		);
		await grabRelease(3);
		const grab = calls.find((c) => c.url === '/api/v1/releases/3/grab');
		expect(grab!.init.body).toBeTruthy();
		expect(grab!.init.body).not.toBe('');
	});
});

describe('the auth posts', () => {
	it('sends setup and login with the JSON content type and the CSRF header', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse({ ...sessionBody, authenticated: false, setup_required: true })
				: jsonResponse(sessionBody)
		);

		await setupOwner('joe', 'correct horse battery');
		await login('joe', 'correct horse battery');

		for (const path of ['/api/v1/auth/setup', '/api/v1/auth/login']) {
			const call = calls.find((c) => c.url === path);
			expect(call, `${path} must be issued`).toBeDefined();
			const headers = headersOf(call!.init);
			expect(headers['content-type']).toBe('application/json');
			expect(headers[CSRF_HEADER.toLowerCase()]).toBe(SESSION_TOKEN);
			expect(JSON.parse(call!.init.body as string)).toEqual({
				username: 'joe',
				password: 'correct horse battery'
			});
		}
	});

	it('reads the server’s own session shape', async () => {
		stubFetch(() =>
			jsonResponse({ authenticated: false, setup_required: true, csrf_token: SESSION_TOKEN })
		);
		expect(await fetchSession()).toEqual({
			authenticated: false,
			setupRequired: true,
			csrfToken: SESSION_TOKEN,
			userId: undefined,
			username: undefined,
			isOwner: false,
			sudoUntil: undefined
		});
	});

	it('drops the token on sign-out so the next sign-in bootstraps a fresh one', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session' ? jsonResponse(sessionBody) : jsonResponse({})
		);
		await fetchSession();
		expect(currentCsrfToken()).toBe(SESSION_TOKEN);
		await logout();
		expect(currentCsrfToken()).toBe('');
	});
});

describe('a 401 sends the browser to sign in', () => {
	it('calls the handler instead of leaving a page of error banners', async () => {
		stubFetch(() =>
			jsonResponse({ error: 'unauthorized', message: 'this request has no session' }, 401)
		);

		const seen = vi.fn();
		onUnauthorized(seen);

		await expect(fetchSession()).rejects.toBeInstanceOf(ApiError);
		expect(seen).toHaveBeenCalledTimes(1);
	});

	it('still throws, so the caller does not carry on as if it had data', async () => {
		stubFetch(() => jsonResponse({ error: 'unauthorized', message: 'expired' }, 401));
		onUnauthorized(vi.fn());
		await expect(fetchSession()).rejects.toMatchObject({ status: 401 });
	});

	it('does not fire on any other failure', async () => {
		stubFetch(() => jsonResponse({ error: 'conflict', message: 'no indexer service' }, 409));
		const seen = vi.fn();
		onUnauthorized(seen);
		await expect(fetchSession()).rejects.toMatchObject({ status: 409 });
		expect(seen).not.toHaveBeenCalled();
	});
});

describe('a 403 from csrfProtected', () => {
	it('re-bootstraps the token and retries exactly once', async () => {
		let issued = 0;
		stubFetch((url) => {
			if (url === '/api/v1/auth/session') {
				issued += 1;
				return jsonResponse({ ...sessionBody, csrf_token: `token-${issued}` });
			}
			const headers = headersOf(calls[calls.length - 1].init);
			if (headers[CSRF_HEADER.toLowerCase()] !== 'token-2') {
				return jsonResponse(
					{ error: 'csrf', message: 'the CSRF token does not match this session' },
					403
				);
			}
			return jsonResponse({ candidate_id: 5, message: 'sent' });
		});

		const result = await grabRelease(5);

		expect(result.candidateId).toBe(5);
		expect(calls.map((c) => c.url)).toEqual([
			'/api/v1/auth/session',
			'/api/v1/releases/5/grab',
			'/api/v1/auth/session',
			'/api/v1/releases/5/grab'
		]);
	});

	it('surfaces a second 403 rather than looping', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse({ error: 'csrf', message: 'this request carries no CSRF token' }, 403)
		);

		await expect(grabRelease(9)).rejects.toMatchObject({ status: 403 });
		expect(calls.filter((c) => c.url === '/api/v1/releases/9/grab')).toHaveLength(2);
	});
});
