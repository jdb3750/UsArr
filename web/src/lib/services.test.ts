import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	ApiError,
	CSRF_HEADER,
	createService,
	deleteService,
	fetchServicesHealth,
	forgetCsrfToken,
	listServices,
	normalizeOrigin,
	onUnauthorized,
	requiresCredentialReentry,
	testNewService,
	testService,
	updateService
} from './api';

/**
 * The services client, asserted against what internal/httpapi/services.go
 * actually accepts and returns.
 *
 * Three properties matter more than the rest and each has its own block:
 *
 *  1. The API key is WRITE-ONLY. The server never returns it, so nothing this
 *     client hands a screen may contain it — asserted by searching the whole
 *     serialised result, not by checking for a field name someone might rename.
 *  2. Changing a base URL's scheme, host or port requires re-entering the key
 *     (reference/security.md §1.6). The server enforces it with a 400; this
 *     client applies the same rule BEFORE the request so a form can say why.
 *  3. PATCH and DELETE go through the same CSRF path POST does. `csrfProtected`
 *     wraps all three identically in server.go and checks the content type
 *     first, so a hand-rolled DELETE answers 415 and looks like a routing bug.
 *
 * cmd/usarr/browserflow_test.go asserts the same shapes against the real server.
 */

type Call = { url: string; init: RequestInit };

const SESSION_TOKEN = 'csrf-token-from-the-server';

/**
 * A key shaped like a real Prowlarr one, and deliberately unlike anything else
 * in this file so a leak assertion cannot pass for the wrong reason.
 */
const API_KEY = 'prowlarrKEY7f3c9a2b5e8d1046c7b2f9e3';

let calls: Call[];

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

/** The exact shape toServiceResponse marshals. Note: no api key, in any form. */
const serviceBody = {
	id: 4,
	kind: 'prowlarr',
	role: 'indexer',
	name: 'Prowlarr',
	base_url: 'http://prowlarr:9696',
	api_version: 'v1',
	enabled: true,
	priority: 0,
	managed_by: 'ui',
	verify_tls: true,
	has_credential: true,
	health_state: 'healthy',
	breaker_state: 'closed'
};

function headersOf(init: RequestInit): Record<string, string> {
	const out: Record<string, string> = {};
	for (const [key, value] of Object.entries((init.headers ?? {}) as Record<string, string>)) {
		out[key.toLowerCase()] = value;
	}
	return out;
}

/** Everything csrfProtected demands of a write, checked in one place. */
function expectCsrfShape(call: Call, method: string) {
	const headers = headersOf(call.init);
	expect(call.init.method).toBe(method);
	expect(headers['content-type']).toBe('application/json');
	expect(headers[CSRF_HEADER.toLowerCase()]).toBe(SESSION_TOKEN);
	expect(typeof call.init.body).toBe('string');
	expect(call.init.credentials).toBe('same-origin');
}

beforeEach(() => {
	forgetCsrfToken();
	onUnauthorized(undefined);
});

afterEach(() => {
	vi.unstubAllGlobals();
	onUnauthorized(undefined);
});

describe('reading the configured services', () => {
	it('maps GET /api/v1/services onto has_credential, never onto a key', async () => {
		stubFetch(() => jsonResponse({ services: [serviceBody] }));

		const services = await listServices();

		expect(calls[0].url).toBe('/api/v1/services');
		expect(services).toHaveLength(1);
		expect(services[0]).toMatchObject({
			id: 4,
			kind: 'prowlarr',
			role: 'indexer',
			name: 'Prowlarr',
			baseUrl: 'http://prowlarr:9696',
			hasCredential: true,
			enabled: true
		});
		// The server sends `has_credential: true` INSTEAD of a masked key, so
		// there is nothing key-shaped anywhere in what a screen receives.
		expect(Object.keys(services[0])).not.toContain('apiKey');
		expect(JSON.stringify(services[0]).toLowerCase()).not.toContain('api_key');
	});

	it('reads the health rows with the verbatim problem and the server’s action', async () => {
		stubFetch(() =>
			jsonResponse({
				services: [
					{
						id: 4,
						name: 'Prowlarr',
						kind: 'prowlarr',
						role: 'indexer',
						base_url: 'http://prowlarr:9696',
						enabled: true,
						state: 'degraded',
						problem:
							'Get "http://prowlarr:9696/api/v1/system/status": dial tcp: connection refused',
						action: 'Test connection',
						breaker_state: 'half-open',
						consecutive_failures: 2,
						warnings: [{ source: 'IndexerStatusCheck', message: 'Indexers unavailable' }],
						blocked_indexers: [{ indexer_id: 2, name: 'Blocked Tracker' }],
						stale: false
					}
				],
				any_unhealthy: true,
				setup_required: false
			})
		);

		const health = await fetchServicesHealth();

		expect(calls[0].url).toBe('/api/v1/services/health');
		expect(health.anyUnhealthy).toBe(true);
		expect(health.setupRequired).toBe(false);
		// Verbatim, not summarised — ARCHITECTURE.md §17.3.
		expect(health.services[0].problem).toContain('connection refused');
		expect(health.services[0].action).toBe('Test connection');
		expect(health.services[0].warnings[0].message).toBe('Indexers unavailable');
		expect(health.services[0].blockedIndexers[0].indexerId).toBe(2);
	});

	it('says setup is required rather than rendering an empty table', async () => {
		stubFetch(() => jsonResponse({ services: [], any_unhealthy: false, setup_required: true }));
		const health = await fetchServicesHealth();
		expect(health.services).toEqual([]);
		expect(health.setupRequired).toBe(true);
	});
});

describe('adding a service', () => {
	it('posts only the fields createServiceRequest declares', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session' ? jsonResponse(sessionBody) : jsonResponse(serviceBody, 201)
		);

		const created = await createService({
			kind: 'prowlarr',
			name: '  Prowlarr  ',
			baseUrl: ' http://prowlarr:9696 ',
			apiKey: API_KEY
		});

		const call = calls.find((c) => c.url === '/api/v1/services')!;
		expectCsrfShape(call, 'POST');

		// decodeJSON runs with DisallowUnknownFields: one extra key is a 400,
		// not an ignored setting.
		const body = JSON.parse(call.init.body as string);
		expect(Object.keys(body).sort()).toEqual(['api_key', 'base_url', 'kind', 'name']);
		expect(body).toEqual({
			kind: 'prowlarr',
			name: 'Prowlarr',
			base_url: 'http://prowlarr:9696',
			api_key: API_KEY
		});

		// The key went UP. Nothing came back down carrying it.
		expect(JSON.stringify(created)).not.toContain(API_KEY);
		expect(created.hasCredential).toBe(true);
	});

	it('surfaces the mandatory connection test’s verbatim failure', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse(
						{
							error: 'connection_test_failed',
							message: 'Prowlarr answered 401 Unauthorized',
							action: 'Test connection'
						},
						502
					)
		);

		const failure = await createService({
			kind: 'prowlarr',
			name: 'Prowlarr',
			baseUrl: 'http://prowlarr:9696',
			apiKey: 'wrong-key'
		}).catch((error: unknown) => error);

		expect(failure).toBeInstanceOf(ApiError);
		const error = failure as ApiError;
		expect(error.status).toBe(502);
		expect(error.code).toBe('connection_test_failed');
		// The screen renders `detail`, which is the server's sentence with no
		// `HTTP nnn:` prefix and no JSON wrapper around it.
		expect(error.detail).toBe('Prowlarr answered 401 Unauthorized');
		expect(error.action).toBe('Test connection');
	});

	it('tests an unsaved instance against POST /api/v1/services/test', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse({
						ok: true,
						reachable: true,
						key_proven_valid: true,
						app_name: 'Prowlarr',
						app_version: '1.30.2.4939',
						message: 'connected'
					})
		);

		const result = await testNewService({
			kind: 'prowlarr',
			name: 'Prowlarr',
			baseUrl: 'http://prowlarr:9696',
			apiKey: API_KEY
		});

		const call = calls.find((c) => c.url === '/api/v1/services/test')!;
		expectCsrfShape(call, 'POST');
		expect(JSON.parse(call.init.body as string)).toEqual({
			kind: 'prowlarr',
			base_url: 'http://prowlarr:9696',
			api_key: API_KEY
		});
		expect(result).toMatchObject({ ok: true, keyProvenValid: true, appName: 'Prowlarr' });
	});
});

/**
 * reference/security.md §1.6. The server refuses with 400
 * credential_reentry_required; this client refuses first, so the form can say
 * why while the user is still typing.
 */
describe('changing a base URL requires re-entering the key', () => {
	it('treats a different scheme, host or port as a change', () => {
		expect(requiresCredentialReentry('http://prowlarr:9696', 'http://prowlarr:9696/')).toBe(false);
		expect(requiresCredentialReentry('http://Prowlarr:9696', 'http://prowlarr:9696')).toBe(false);
		// A scheme-only edit changes the AAD, so it changes the answer. Comparing
		// host:port alone would let this through and leave an unopenable key.
		expect(requiresCredentialReentry('https://nas:443', 'http://nas:443')).toBe(true);
		expect(requiresCredentialReentry('http://prowlarr:9696', 'http://attacker.example:9696')).toBe(
			true
		);
		expect(requiresCredentialReentry('http://prowlarr:9696', 'http://prowlarr:9697')).toBe(true);
		// Default ports are filled in on both sides, exactly as NormalizeHostPort does.
		expect(requiresCredentialReentry('http://prowlarr', 'http://prowlarr:80')).toBe(false);
		expect(requiresCredentialReentry('https://prowlarr', 'https://prowlarr:443')).toBe(false);
		// Fails closed: something neither side can parse counts as changed.
		expect(requiresCredentialReentry('http://prowlarr:9696', 'not a url')).toBe(true);
		// An empty edit is not an edit.
		expect(requiresCredentialReentry('http://prowlarr:9696', '  ')).toBe(false);
		expect(normalizeOrigin('ftp://prowlarr:9696')).toBeUndefined();
	});

	it('refuses the PATCH locally instead of letting the user hit a 400', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session' ? jsonResponse(sessionBody) : jsonResponse(serviceBody)
		);

		const failure = await updateService(4, {
			baseUrl: 'http://elsewhere:9696',
			currentBaseUrl: 'http://prowlarr:9696'
		}).catch((error: unknown) => error);

		expect(failure).toBeInstanceOf(ApiError);
		expect(failure as ApiError).toMatchObject({
			status: 400,
			code: 'credential_reentry_required',
			action: 'Re-enter the API key'
		});
		// And the request was never issued, so nothing was sent to the new host.
		expect(calls.filter((c) => c.url === '/api/v1/services/4')).toHaveLength(0);
	});

	it('sends the new address together with the re-entered key', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse({ ...serviceBody, base_url: 'http://elsewhere:9696' })
		);

		await updateService(4, {
			name: 'Prowlarr',
			baseUrl: 'http://elsewhere:9696',
			apiKey: API_KEY,
			currentBaseUrl: 'http://prowlarr:9696'
		});

		const call = calls.find((c) => c.url === '/api/v1/services/4')!;
		expectCsrfShape(call, 'PATCH');
		expect(JSON.parse(call.init.body as string)).toEqual({
			name: 'Prowlarr',
			base_url: 'http://elsewhere:9696',
			api_key: API_KEY
		});
	});

	it('omits api_key entirely when the address is unchanged, so the stored one is kept', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session' ? jsonResponse(sessionBody) : jsonResponse(serviceBody)
		);

		await updateService(4, {
			name: 'Renamed',
			baseUrl: 'http://prowlarr:9696/',
			apiKey: '   ',
			currentBaseUrl: 'http://prowlarr:9696'
		});

		const body = JSON.parse(calls.find((c) => c.url === '/api/v1/services/4')!.init.body as string);
		// The trailing slash travels as typed: normalizeBaseURL trims it
		// server-side, and a client that also trimmed would be a second, quietly
		// disagreeing normaliser.
		expect(body).toEqual({ name: 'Renamed', base_url: 'http://prowlarr:9696/' });
		expect(Object.keys(body)).not.toContain('api_key');
	});

	it('applies the same rule to the connection test', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse({ ok: true, reachable: true, key_proven_valid: true, message: 'connected' })
		);

		await expect(
			testService(4, {
				baseUrl: 'http://attacker.example:9696',
				currentBaseUrl: 'http://prowlarr:9696'
			})
		).rejects.toMatchObject({ code: 'credential_reentry_required' });
		expect(calls.filter((c) => c.url === '/api/v1/services/4/test')).toHaveLength(0);

		// Re-testing the saved instance sends an empty object: an empty api_key
		// means "use the stored one", which the server allows only because the
		// host is unchanged.
		const result = await testService(4);
		const call = calls.find((c) => c.url === '/api/v1/services/4/test')!;
		expectCsrfShape(call, 'POST');
		expect(JSON.parse(call.init.body as string)).toEqual({});
		expect(result.ok).toBe(true);
	});
});

describe('DELETE and PATCH take the same CSRF path POST does', () => {
	it('sends DELETE with the JSON content type and the token', async () => {
		stubFetch((url, init) => {
			if (url === '/api/v1/auth/session') return jsonResponse(sessionBody);
			// csrfProtected checks the content type BEFORE the token, so a
			// hand-rolled DELETE fails here with 415 rather than 403.
			const headers = headersOf(init);
			if (headers['content-type'] !== 'application/json') {
				return jsonResponse({ error: 'unsupported_media_type', message: 'must be JSON' }, 415);
			}
			if (headers[CSRF_HEADER.toLowerCase()] !== SESSION_TOKEN) {
				return jsonResponse({ error: 'csrf', message: 'no token' }, 403);
			}
			return new Response(null, { status: 204 });
		});

		await expect(deleteService(4)).resolves.toBeUndefined();

		const call = calls.find((c) => c.url === '/api/v1/services/4')!;
		expectCsrfShape(call, 'DELETE');
		// A body it does not need, so the header rule is one rule and not a
		// per-method exception.
		expect(call.init.body).toBe('{}');
	});

	it('re-bootstraps the token and retries once on a stale-token 403', async () => {
		let issued = 0;
		stubFetch((url) => {
			if (url === '/api/v1/auth/session') {
				issued += 1;
				return jsonResponse({ ...sessionBody, csrf_token: `token-${issued}` });
			}
			const headers = headersOf(calls[calls.length - 1].init);
			if (headers[CSRF_HEADER.toLowerCase()] !== 'token-2') {
				return jsonResponse({ error: 'csrf', message: 'the token does not match' }, 403);
			}
			return new Response(null, { status: 204 });
		});

		await deleteService(7);

		expect(calls.map((c) => c.url)).toEqual([
			'/api/v1/auth/session',
			'/api/v1/services/7',
			'/api/v1/auth/session',
			'/api/v1/services/7'
		]);
	});

	it('does NOT retry a sudo_required 403 — re-bootstrapping cannot fix it', async () => {
		stubFetch((url) =>
			url === '/api/v1/auth/session'
				? jsonResponse(sessionBody)
				: jsonResponse(
						{
							error: 'sudo_required',
							message:
								'this operation touches a stored credential and needs a fresh password confirmation',
							action: 'Confirm your password'
						},
						403
					)
		);

		await expect(deleteService(4)).rejects.toMatchObject({ status: 403, code: 'sudo_required' });
		// Once, not twice: the fix is a password confirmation, not a new token.
		expect(calls.filter((c) => c.url === '/api/v1/services/4')).toHaveLength(1);
	});
});

describe('no response the client receives can carry the API key', () => {
	it('holds across list, create, patch and test', async () => {
		// A server that (wrongly) echoed the key everywhere it could. The client
		// must still hand the screen nothing containing it, because it reads only
		// the fields internal/httpapi declares — and none of them is the key.
		const leaky = {
			...serviceBody,
			api_key: API_KEY,
			apiKey: API_KEY,
			credential: API_KEY
		};
		stubFetch((url) => {
			if (url === '/api/v1/auth/session') return jsonResponse(sessionBody);
			if (url === '/api/v1/services' && calls[calls.length - 1].init.method === undefined) {
				return jsonResponse({ services: [leaky] });
			}
			if (url === '/api/v1/services/4/test') {
				return jsonResponse({
					ok: true,
					reachable: true,
					key_proven_valid: true,
					message: 'connected',
					api_key: API_KEY
				});
			}
			return jsonResponse(leaky);
		});

		const received: unknown[] = [
			await listServices(),
			await createService({
				kind: 'prowlarr',
				name: 'Prowlarr',
				baseUrl: 'http://prowlarr:9696',
				apiKey: API_KEY
			}),
			await updateService(4, { name: 'Renamed' }),
			await testService(4)
		];

		for (const value of received) {
			expect(JSON.stringify(value)).not.toContain(API_KEY);
		}
	});
});
