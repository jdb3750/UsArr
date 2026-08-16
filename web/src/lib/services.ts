/**
 * The Services screen's vocabulary, as pure functions.
 *
 * Everything here is DOM-free and deterministic so the node-environment vitest
 * run can pin it. The screen itself (`routes/services/+page.svelte`) does the
 * rendering; this file owns the decisions that ARCHITECTURE.md §17.3 makes
 * rules rather than taste, because a rule that lives only inside a `{#if}` in a
 * template is a rule nothing can test.
 *
 * THE THREE COLUMN CONTRACTS, and they are not interchangeable:
 *
 *   Problem  is the upstream's own words, VERBATIM, or `—`. One object per
 *            cell. It never carries a rendering decision, a rebuttal or a
 *            sentence beginning "Nothing is wrong." — that inverts the meaning
 *            of the one column a user scans for what is broken.
 *
 *   State    is UsArr speaking, in plain language. The verbatim rule stops at
 *            Problem. So `paused` with `7 failed attempts, retrying 14:19`
 *            under it, never `degraded / breaker open`; `this may be a
 *            different Prowlarr`, never `needs re-identification`. The
 *            implementation vocabulary is real and valuable and it lives behind
 *            the row expander, which `mechanics()` builds.
 *
 *   Action   is the one button that fixes it, or `—`. Never `No action needed`.
 *
 * WHAT THE SERVER DOES NOT SEND, stated here so no caller invents it:
 *   · GET /api/v1/services/health carries no `url_base`. Only GET
 *     /api/v1/services does, so the screen reads both and joins on id.
 *   · Nothing carries a per-instance item count, a sync-channel time or a
 *     clock skew. v0.1 has one service kind — prowlarr, an indexer with no
 *     catalogue — so `Items` and `Last successful sync` are `Not applicable`
 *     for every row that exists, which is the honest answer rather than a
 *     zero. `describeSync()` carries the channel vocabulary §17.3 specifies so
 *     the first catalogue source does not have to reinvent it; nothing in v0.1
 *     reaches it.
 */

import { NOTHING } from './list';
import type { ServiceHealth, ServiceInstance } from './api';

/** One row of the health table: the health row, plus what only GET /services has. */
export interface ServiceRow {
	health: ServiceHealth;
	instance?: ServiceInstance;
}

/** The status colour roles app.css exposes as `.st--ok` / `--warn` / `--err` / `--none`. */
export type Tone = 'ok' | 'warn' | 'err' | 'none';

/** What the `State` cell renders: UsArr's own word, plus one qualifying line. */
export interface StateLabel {
	/** The plain-language word. Never ecosystem or implementation vocabulary. */
	word: string;
	tone: Tone;
	/** The qualifier under it, or '' when the word says everything. */
	detail: string;
	/** Which icon carries the tone. Text is always present, so the glyph is decorative. */
	icon: 'check' | 'alert' | 'x-circle' | 'dash-circle';
}

const ICON_FOR: Record<Tone, StateLabel['icon']> = {
	ok: 'check',
	warn: 'alert',
	err: 'x-circle',
	none: 'dash-circle'
};

/** `14:19`, 24-hour, en-GB. Empty for anything unparseable. */
export function clockOf(iso: string | undefined): string {
	const at = toDate(iso);
	if (!at) return '';
	return new Intl.DateTimeFormat('en-GB', {
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	}).format(at);
}

/** `8 Aug 2026` — one date format, no leading zero, always with the year (§9.1). */
export function dateOf(iso: string | undefined): string {
	const at = toDate(iso);
	if (!at) return '';
	return new Intl.DateTimeFormat('en-GB', {
		day: 'numeric',
		month: 'short',
		year: 'numeric'
	}).format(at);
}

function toDate(iso: string | undefined): Date | undefined {
	if (!iso) return undefined;
	const at = new Date(iso);
	return Number.isNaN(at.getTime()) ? undefined : at;
}

/**
 * The relative half of §9.1's rule that every user-facing timestamp carries
 * both forms. `6 minutes ago`, `in 4 minutes`, `1 day ago`.
 *
 * Rounded to the coarsest unit that still says something, because `in 247
 * seconds` is a number a human has to convert.
 */
export function relativeTo(iso: string | undefined, now: Date): string {
	const at = toDate(iso);
	if (!at) return '';
	const seconds = Math.round((at.getTime() - now.getTime()) / 1000);
	const ahead = seconds > 0;
	const magnitude = Math.abs(seconds);

	let value: number;
	let unit: string;
	if (magnitude < 45) {
		return ahead ? 'in under a minute' : 'just now';
	} else if (magnitude < 3600) {
		value = Math.round(magnitude / 60);
		unit = 'minute';
	} else if (magnitude < 86400) {
		value = Math.round(magnitude / 3600);
		unit = 'hour';
	} else {
		value = Math.round(magnitude / 86400);
		unit = 'day';
	}
	const plural = value === 1 ? unit : `${unit}s`;
	return ahead ? `in ${value} ${plural}` : `${value} ${plural} ago`;
}

/**
 * `14:02, 6 minutes ago`, and `11:47 on 15 Aug 2026, 1 day ago` once it is more
 * than a day old — §9.1: "Every user-facing timestamp carries the relative
 * form, and past 24 hours it carries a date."
 */
export function stampOf(iso: string | undefined, now: Date): string {
	const at = toDate(iso);
	if (!at) return '';
	const relative = relativeTo(iso, now);
	const olderThanADay = Math.abs(now.getTime() - at.getTime()) >= 86400 * 1000;
	const absolute = olderThanADay ? `${clockOf(iso)} on ${dateOf(iso)}` : clockOf(iso);
	return relative ? `${absolute}, ${relative}` : absolute;
}

/**
 * THE STATE COLUMN, in UsArr's own words.
 *
 * The API's `state` is one of `healthy`, `degraded`, `down`,
 * `needs re-identification` or `unknown` (internal/httpapi/services.go). Those
 * are the mechanism's names; §17.3 is explicit that this column is not where
 * they belong. Every mapping below turns one into something a person who has
 * never read the architecture can act on, and the mechanism survives intact in
 * `mechanics()` behind the row expander.
 *
 * `kind` is threaded through because "this may be a different Sonarr" names the
 * application. A generic "this may be a different instance" is the sentence
 * §17.3 rejected.
 */
export function stateLabel(row: ServiceRow, now: Date): StateLabel {
	const h = row.health;
	const paused = h.breakerState.toLowerCase() === 'open';
	const failures = h.consecutiveFailures;

	if (h.state === 'needs re-identification') {
		return label(`this may be a different ${applicationName(h.kind)}`, 'err', 'sync is paused');
	}
	if (!h.enabled) {
		return label('turned off', 'none', 'UsArr is not contacting it');
	}
	if (paused) {
		return label('paused', 'err', pausedDetail(failures, h.breakerRetryAt, now));
	}
	if (h.state === 'down') {
		return label('not answering', 'err', attempts(failures));
	}
	if (h.state === 'degraded') {
		return label('some attempts are failing', 'warn', attempts(failures));
	}
	// STALE OUTRANKS `healthy`, AND THAT IS THE POINT OF THE FLAG.
	//
	// handleServicesHealth answers `healthy` on the strength of a stored
	// `last_ok_at` alone, so a service that answered once when it was added and
	// has not been contacted since arrives here as `healthy`, `stale: true`. That
	// is a fact about the past, and rendering it as a green tick claims a
	// measurement UsArr has not taken. §10's rule for the column is that it says
	// what was observed, and what was observed is nothing.
	if (h.stale) {
		return label('not checked yet', 'none', 'no probe has run');
	}
	if (h.state === 'healthy') {
		return label('healthy', 'ok', '');
	}
	// `unknown` with a probe behind it: the probe ran and could not classify the
	// instance. An honest "not measured" still beats a green tick.
	return label('not checked yet', 'none', '');
}

function label(word: string, tone: Tone, detail: string): StateLabel {
	return { word, tone, detail, icon: ICON_FOR[tone] };
}

function attempts(failures: number): string {
	if (failures <= 0) return '';
	return failures === 1 ? '1 failed attempt' : `${failures} failed attempts`;
}

function pausedDetail(failures: number, retryAt: string | undefined, now: Date): string {
	const tried = attempts(failures);
	const clock = clockOf(retryAt);
	if (!clock) return tried || 'waiting before the next attempt';
	const relative = relativeTo(retryAt, now);
	const when = relative ? `retrying ${clock}, ${relative}` : `retrying ${clock}`;
	return tried ? `${tried}, ${when}` : when;
}

/**
 * The application's own name, capitalised, for a sentence about it.
 *
 * SERVICE_KINDS is one entry today. The map is a lookup rather than a
 * capitalise() because `audiobookshelf` is `Audiobookshelf` and `lazylibrarian`
 * is `LazyLibrarian`, and a rule derived from the first three would be wrong on
 * the fourth.
 */
const APPLICATION_NAMES: Record<string, string> = {
	prowlarr: 'Prowlarr',
	sonarr: 'Sonarr',
	radarr: 'Radarr',
	lidarr: 'Lidarr',
	navidrome: 'Navidrome',
	jellyfin: 'Jellyfin',
	audiobookshelf: 'Audiobookshelf',
	kavita: 'Kavita',
	komga: 'Komga',
	lazylibrarian: 'LazyLibrarian'
};

export function applicationName(kind: string): string {
	const key = kind.trim().toLowerCase();
	return APPLICATION_NAMES[key] ?? (key === '' ? 'instance' : key);
}

/**
 * THE IMPLEMENTATION VOCABULARY, which §17.3 keeps and moves rather than
 * deletes: `State OPEN · Next probe 14:19, in 4 minutes · Consecutive failures
 * 7`. It is precise, it is what a bug report needs, and it belongs behind the
 * expander instead of in the column a user scans.
 *
 * Returned as parts so the caller can render `·` as a real element rather than
 * as generated content, which would land inside the accessible name.
 */
export function mechanics(row: ServiceRow, now: Date): string[] {
	const h = row.health;
	const parts: string[] = [];
	if (h.breakerState) parts.push(`State ${h.breakerState.toUpperCase()}`);
	if (h.breakerRetryAt) {
		const relative = relativeTo(h.breakerRetryAt, now);
		parts.push(`Next probe ${clockOf(h.breakerRetryAt)}${relative ? `, ${relative}` : ''}`);
	}
	parts.push(`Consecutive failures ${h.consecutiveFailures}`);
	if (h.observedAt) parts.push(`Last probed ${stampOf(h.observedAt, now)}`);
	else parts.push('Never probed');
	return parts;
}

/** What a cell shows when there is a value, and what it shows when there is not. */
export interface Cell {
	text: string;
	/** The muted second line, or ''. */
	sub: string;
	/** True when `text` is one of §9.1's three nothing-words, so the cell mutes. */
	muted: boolean;
}

function nothing(text: string, sub = ''): Cell {
	return { text, sub, muted: true };
}

/**
 * A sync channel, as §17.3 requires it to be labelled.
 *
 * `ordered` is the load-bearing field, not `label`: a source with no ordering
 * guarantee has no delta at all, and §17.3 forbids rendering its freshness
 * number with the same weight as one that does. So the type makes the
 * distinction a caller cannot forget to pass.
 */
export interface SyncChannel {
	/** `delta`, `page-walk delta`, `full`, `reconcile`. */
	kind: 'delta' | 'page-walk delta' | 'full' | 'reconcile';
	at: string;
	/** False for a source that publishes no change feed at all (§7.1a). */
	ordered: boolean;
}

/**
 * The `Last successful sync` cell for a source that HAS a catalogue.
 *
 * ⚠️ NOTHING IN v0.1 REACHES THIS, and it is written now because it cannot be
 * retrofitted: the labelling rule is the whole point of the column, and the
 * first catalogue source must not have to rediscover it. v0.1's only kind is
 * prowlarr, an indexer, so `syncCell()` below answers `Not applicable` for
 * every row that can exist today.
 *
 *   ordered channel      →  `delta 14:02` / `6 minutes ago`
 *   channel 3b (§7.1a)   →  `page-walk delta 13:40` / `28 minutes ago`
 *   no ordering at all   →  `no change feed — full compare at 09:12`
 */
export function describeSync(channel: SyncChannel, now: Date): Cell {
	const clock = clockOf(channel.at);
	if (!channel.ordered) {
		return {
			text: `no change feed — full compare at ${clock}`,
			sub: relativeTo(channel.at, now),
			muted: false
		};
	}
	return { text: `${channel.kind} ${clock}`, sub: relativeTo(channel.at, now), muted: false };
}

/**
 * The `Last successful sync` cell for a real row.
 *
 * An indexer has no catalogue, so the concept does not exist for it — which is
 * exactly §9.1's `Not applicable`, and NOT `—` and NOT `0`. `Never` is kept as
 * a real answer for a catalogue source that has never synced.
 */
export function syncCell(row: ServiceRow): Cell {
	if (isIndexer(row.health)) return nothing(NOTHING.inapplicable, 'no catalogue');
	return nothing('Never');
}

/**
 * The `Items` cell: how many works this instance contributes.
 *
 * Same reasoning, and the same answer for the same reason: an indexer
 * contributes no works, so the column is inapplicable rather than zero. A zero
 * would read as "it has a catalogue and it is empty", which is a different and
 * false claim.
 */
export function itemsCell(row: ServiceRow): Cell {
	if (isIndexer(row.health)) return nothing(NOTHING.inapplicable);
	return nothing(NOTHING.empty);
}

export function isIndexer(health: ServiceHealth): boolean {
	return health.role === 'indexer' || health.kind === 'prowlarr';
}

/**
 * The `Problem` cell. One object: what the upstream said, or `—`.
 *
 * A one-line summary is what fits a cell; the full text is rendered untouched
 * in the expander, in mono, wrapped and never truncated. The summary is a
 * PREFIX of the upstream text rather than a paraphrase, so the two cannot
 * disagree, and the full string always rides on `title`.
 */
export function problemCell(row: ServiceRow): Cell {
	const problem = row.health.problem?.trim() ?? '';
	if (problem === '') return nothing(NOTHING.empty);
	return { text: firstLine(problem), sub: '', muted: false };
}

/** The first line of a multi-line upstream body, trimmed. Never a rewrite of it. */
export function firstLine(text: string): string {
	const line = text.split('\n')[0].trim();
	return line === '' ? text.trim() : line;
}

/**
 * How the three 403s are told apart.
 *
 * internal/httpapi answers 403 with THREE codes on the body's `error` field and
 * only one of them is fixable by retrying, so this is an allow-list keyed on
 * the code and NEVER on the status. Blind-retrying a 403 as a stale CSRF token
 * is a bug this codebase has already fixed once; an unknown code surfaces.
 */
export type AuthFailure = 'sudo' | 'forbidden' | 'csrf' | 'other';

export function classify403(code: string): AuthFailure {
	switch (code) {
		case 'sudo_required':
			return 'sudo';
		case 'forbidden':
			return 'forbidden';
		case 'csrf':
			return 'csrf';
		default:
			return 'other';
	}
}

/**
 * `URL base` normalisation, run on blur.
 *
 * §17.3.1: the form NORMALISES rather than only complaining — it adds the
 * leading slash and trims the trailing one — and reports an error only for
 * something it cannot repair. A scheme, a host or a query string is a different
 * value that happens to have been typed here, and silently rewriting one into a
 * path would save the wrong thing.
 *
 * The server owns the rule and enforces it again (normalizeServiceURLBase); this
 * is the form's half, so the user sees the repair while they are still looking
 * at the field.
 */
export interface UrlBaseFix {
	value: string;
	/** '' when the value is usable. Otherwise what cannot be repaired. */
	problem: string;
}

export function normaliseUrlBase(raw: string): UrlBaseFix {
	const value = raw.trim();
	if (value === '') return { value: '', problem: '' };
	if (/^[a-z][a-z0-9+.-]*:\/\//i.test(value)) {
		return {
			value,
			problem:
				'This is a whole address, not a sub-path. Put the scheme and host in Base URL and leave only the path here, for example /prowlarr'
		};
	}
	if (value.includes('?') || value.includes('#')) {
		return { value, problem: 'A URL base is a path. Remove the query string or fragment.' };
	}
	let fixed = value.startsWith('/') ? value : `/${value}`;
	while (fixed.length > 1 && fixed.endsWith('/')) fixed = fixed.slice(0, -1);
	if (fixed === '/') return { value: '', problem: '' };
	return { value: fixed, problem: '' };
}

/**
 * What a failed connection test most likely means, as prose.
 *
 * §17.3 requires the verbatim upstream text PLUS "the two or three most likely
 * causes for that error class": a 404 names the URL base, a TLS error names the
 * pin, a refused connection names the port. This is the mapping, and it is
 * keyed on the upstream text because that is the only signal the API gives —
 * `ConnectionTestResult` carries no error class.
 *
 * The default arm is deliberately not empty: a failure with no recognised
 * shape still gets the two causes that are true of every failure from a
 * container, which is the pair a first-time user needs most.
 */
export function likelyCauses(message: string, kind: string): string[] {
	const text = message.toLowerCase();
	const app = applicationName(kind);
	const fromHere = `UsArr calls the service from the host it runs on, not from your browser, so an address that works in a browser tab may not resolve here.`;

	if (/\b404\b|not found/.test(text)) {
		return [
			`A missing URL base. A reverse proxy serving ${app} under a path needs that path in the URL base field, not in the base URL.`,
			`The wrong kind. Sonarr and Radarr both answer /api/v3/system/status, so a URL under the wrong kind connects and then reports the wrong application.`,
			fromHere
		];
	}
	if (/certificate|x509|tls|ssl/.test(text)) {
		return [
			`The certificate does not match the pinned fingerprint, or it is self-signed and nothing has been pinned yet.`,
			`The address is https but the service is serving plain HTTP, or the other way around.`,
			fromHere
		];
	}
	if (/refused|no such host|dial tcp|timeout|timed out|deadline|unreachable/.test(text)) {
		return [
			`The wrong port. ${app} listens on its own port and it is not the one the reverse proxy uses.`,
			`The service is not running, or a firewall between this host and it is dropping the connection.`,
			fromHere
		];
	}
	if (/401|unauthorized|forbidden|api key/.test(text)) {
		return [
			`The API key is wrong or has been regenerated. It is ${app}'s Settings, General, API Key.`,
			`The key belongs to a different instance. Two ${app} instances have two different keys.`
		];
	}
	return [
		`The address is right but nothing at it is ${app}. The success panel names what answered, which is the only thing that catches a URL pasted under the wrong kind.`,
		fromHere
	];
}

/**
 * The `connected` panel's headline.
 *
 * ⚠️ §17.3 asks for the probed application, its version, its API path AND ONE
 * COUNT, and the count is the one part that is not on the wire: `testResponse`
 * (internal/httpapi/services.go) carries ok, reachable, key_proven_valid,
 * app_name, app_version, api_version, message and action, and nothing that
 * counts anything. It is NOT invented here. The clause that catches "the kind
 * said sonarr and I pasted a Radarr URL" is the application name, and that one
 * is present.
 */
export function connectedHeadline(
	appName: string | undefined,
	appVersion: string | undefined,
	apiVersion: string | undefined
): string {
	const application = [appName, appVersion].filter((p) => p && p !== '').join(' ');
	if (application === '' && !apiVersion) return 'Connected';
	if (!apiVersion) return `${application} answered`;
	if (application === '') return `Something answered on ${apiVersion}`;
	return `${application} answered on ${apiVersion}`;
}

/**
 * WHAT THE CONNECTION TEST ACTUALLY PROVED, which is not always what `ok: true`
 * looks like.
 *
 * `ok: true` with `key_proven_valid: false` is a real and common answer: an
 * instance with `AuthenticationRequired = DisabledForLocalAddresses` returns 200
 * to a request from a local address WITH NO KEY AT ALL, so the 200 proved the
 * host is reachable and proved nothing whatsoever about the credential. Drawing
 * that as a flat "Connected" is a claim UsArr did not observe, and the user acts
 * on it — they save a service with a wrong key and meet it later as a health
 * failure with no visible cause.
 *
 * So `ok` is two outcomes, not one, and the caller renders them with different
 * confidence. The server writes the wording for the unverified case itself
 * (toTestResponse, and the tester's own message); it is used as it stands rather
 * than replaced with something cheerier.
 */
export type TestOutcome = 'connected' | 'reachable' | 'failed';

export function testOutcome(result: { ok: boolean; keyProvenValid: boolean }): TestOutcome {
	if (!result.ok) return 'failed';
	return result.keyProvenValid ? 'connected' : 'reachable';
}

/** The panel title for each outcome. UsArr's own words; the body stays verbatim. */
export function testTitle(outcome: TestOutcome): string {
	switch (outcome) {
		case 'connected':
			return 'Connected, and the API key was accepted';
		case 'reachable':
			return 'Reachable. The API key was not verified';
		default:
			return 'Could not connect';
	}
}

/** The tone each outcome may be drawn in. `reachable` is never `ok`. */
export function testTone(outcome: TestOutcome): Tone {
	switch (outcome) {
		case 'connected':
			return 'ok';
		case 'reachable':
			return 'warn';
		default:
			return 'err';
	}
}

/**
 * The roll-up's rows. §17.3: "A problem is stated canonically once per screen",
 * so this carries a title and a link to the row and NEVER a second copy of the
 * row's action button — there is one place the fix is pressed.
 */
export interface RollupEntry {
	id: number;
	name: string;
	tone: Tone;
	title: string;
}

export function rollup(rows: ServiceRow[], now: Date): RollupEntry[] {
	const out: RollupEntry[] = [];
	for (const row of rows) {
		const state = stateLabel(row, now);
		if (state.tone === 'ok') continue;
		if (state.tone === 'none' && row.health.enabled && !row.health.stale) continue;
		const problem = row.health.problem?.trim() ?? '';
		out.push({
			id: row.health.id,
			name: row.health.name,
			tone: state.tone,
			// UsArr's own words in the title; the verbatim text stays in the row's
			// Problem cell and in its expander, which is the one canonical place.
			title:
				problem === '' ? `${row.health.name} is ${state.word}` : `${row.health.name}: ${state.word}`
		});
	}
	return out;
}

/** `1 error, 2 warnings`, or '' when there is nothing to count. */
export function rollupCount(entries: RollupEntry[]): string {
	const errors = entries.filter((e) => e.tone === 'err').length;
	const warnings = entries.length - errors;
	const parts: string[] = [];
	if (errors > 0) parts.push(errors === 1 ? '1 error' : `${errors} errors`);
	if (warnings > 0) parts.push(warnings === 1 ? '1 warning' : `${warnings} warnings`);
	return parts.join(', ');
}
