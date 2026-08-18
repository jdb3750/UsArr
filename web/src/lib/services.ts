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
 *   · It carries no SYNC-CHANNEL time and no clock skew. `describeSync()`
 *     holds the channel vocabulary §17.3 specifies so the first catalogue
 *     source does not have to reinvent it, and nothing on the wire reaches it
 *     yet: the health row says WHEN a full import last completed, never by
 *     which channel.
 *
 * ⚠️ THIS BLOCK USED TO SAY "Nothing carries a per-instance item count", and
 * that `Items` and `Last successful sync` were `Not applicable` on every row
 * that could exist. Both are false now. The endpoint gained
 * `last_full_sync_at` and `work_count`
 * (internal/httpapi/services.go:655 and :660), and the two cells below render
 * them. The `Not applicable` answer survives for an INDEXER, which is a
 * statement about `role` and not about the wire.
 *
 * THE TWO FRESHNESS FIELDS ARE ONE FACT IN TWO SLOTS, and every call site here
 * reads them as a pair. docs/reference/http-api.md §3.2 is the contract; the
 * short form is that `null` and `0` are independent, so all four combinations
 * are reachable and three of them are NOT "nothing here":
 *
 *   null  + 0   → never synced
 *   time  + 0   → synced, and the upstream really is empty
 *   time  + >0  → the ordinary case
 *   null  + >0  → a partial import's committed batches stand
 *
 * The fourth is why `work_count` is not nulled to match the timestamp, and why
 * nothing here may "tidy" a null timestamp into "no data".
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
	const absolute = absoluteOf(iso, now);
	if (!absolute) return '';
	const relative = relativeTo(iso, now);
	return relative ? `${absolute}, ${relative}` : absolute;
}

/**
 * The absolute half of §9.1's pair, on its own: `14:02`, or `11:47 on 15 Aug
 * 2026` once it is more than a day old.
 *
 * It exists because a TABLE CELL has two slots where a sentence has one.
 * `stampOf` joins the two halves with a comma for a `<li>` or a `title`; a cell
 * puts the absolute half on the value line and the relative half on
 * `.cell-sub`, which is what `describeSync()` already does with `delta 14:02` /
 * `6 minutes ago`. Both callers get the SAME absolute rule from here rather
 * than a second one, so the column cannot drift from the expander above it.
 */
export function absoluteOf(iso: string | undefined, now: Date): string {
	const at = toDate(iso);
	if (!at) return '';
	const olderThanADay = Math.abs(now.getTime() - at.getTime()) >= 86400 * 1000;
	return olderThanADay ? `${clockOf(iso)} on ${dateOf(iso)}` : clockOf(iso);
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
 * The map is a lookup rather than a capitalise() because `audiobookshelf` is
 * `Audiobookshelf` and `lazylibrarian` is `LazyLibrarian`, and a rule derived
 * from the first three would be wrong on the fourth. It deliberately covers more
 * kinds than SERVICE_KINDS accepts, so a row written by a later build renders
 * with a name rather than a slug.
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
 * FIVE ANSWERS, and the last four are the wire's four-state pair:
 *
 *   indexer      →  `Not applicable` / `no catalogue`
 *   null  +  0   →  `Never`
 *   null  + >0   →  `Partial import` / `some rows landed`
 *   time  +  0   →  `14:02` / `6 minutes ago`
 *   time  + >0   →  `14:02` / `6 minutes ago`
 *
 * An indexer has no catalogue, so the concept does not exist for it — which is
 * exactly §9.1's `Not applicable`, and NOT `—` and NOT `0`. That branch is
 * decided on `role`, never on the two fields, because a Prowlarr reports
 * `null` / `0` exactly like an unsynced catalogue source and only `role`
 * separates them (http-api.md §3.2).
 *
 * ⚠️ `Partial import` IS NOT A NOTHING-WORD, and it is deliberately not muted.
 * It is the one state where the two columns disagree on purpose: this cell says
 * no import has ever completed while `Items` shows a real count, because a
 * partial import leaves its committed batches behind (internal/libsync's
 * FullImport). Rendering it as `Never` would make the count next to it look
 * like a contradiction; muting it would hide the one row on the screen whose
 * freshness is unknown.
 */
export function syncCell(row: ServiceRow, now: Date): Cell {
	if (isIndexer(row.health)) return nothing(NOTHING.inapplicable, 'no catalogue');

	const { lastFullSyncAt, workCount } = row.health;
	// `absoluteOf` is the parse as well as the format: an unparseable timestamp
	// yields '' and falls through to the never/partial branch rather than
	// rendering an empty cell that claims a sync happened.
	const absolute = absoluteOf(lastFullSyncAt ?? undefined, now);
	if (absolute === '') {
		if (workCount > 0) {
			return { text: 'Partial import', sub: 'some rows landed', muted: false };
		}
		return nothing('Never');
	}
	return {
		text: absolute,
		sub: relativeTo(lastFullSyncAt ?? undefined, now),
		muted: false
	};
}

/**
 * The `Items` cell: how many DISTINCT WORKS this instance contributes.
 *
 * ⚠️ NOT the Libraries screen's per-library `item_count`, and the copy must
 * never imply otherwise. REVIEW-LOG LS-115 states the separation: that number
 * counts `library_member` rows PER LIBRARY and is EDITION-grained, this one
 * counts distinct `work` rows PER SERVICE INSTANCE. A user-defined library
 * binds containers across instances (§17.8), so neither grouping nor grain
 * lines up. The two are equal today only by coincidence — one writer, one
 * catalogue source, one `edition_id = 0` sentinel — and LS-115 is explicit that
 * the coincidence is not something either query may be read as asserting. So
 * `Items` here says what it counts and never borrows the other screen's word.
 *
 * FOUR ANSWERS:
 *
 *   indexer      →  `Not applicable`   an indexer contributes no works at all
 *   null  +  0   →  `—`                nothing has ever been counted
 *   time  +  0   →  `0`                counted, and the upstream really is empty
 *   any   + >0   →  the count
 *
 * ⚠️ `—` AND `0` ARE DIFFERENT ANSWERS HERE, which is the whole reason the
 * server sends the timestamp alongside the count. `0` is a MEASUREMENT — an
 * import completed and found nothing — and rendering the never-synced case as
 * `0` too would state that measurement without having taken it. An indexer gets
 * neither: `Not applicable`, because a zero there would read as "it has a
 * catalogue and it is empty", a claim about a catalogue that does not exist.
 */
export function itemsCell(row: ServiceRow): Cell {
	if (isIndexer(row.health)) return nothing(NOTHING.inapplicable);

	const { lastFullSyncAt, workCount } = row.health;
	// A partial import's committed rows are real and are shown, so the count
	// wins over the missing timestamp whenever there is one to show.
	if (workCount <= 0 && lastFullSyncAt === null) return nothing(NOTHING.empty);
	return { text: workCount.toLocaleString('en-GB'), sub: '', muted: false };
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
	const path = apiPathOf(apiVersion);
	if (application === '' && path === '') return 'Connected';
	if (path === '') return `${application} answered`;
	if (application === '') return `Something answered on ${path}`;
	return `${application} answered on ${path}`;
}

/**
 * The API PATH, which is what §17.3 asks the panel to show and is not quite
 * what the wire carries.
 *
 * `api_version` is `APIInfoResource.current`, and the *Arrs put the bare
 * version in it — Prowlarr answers `v1`, Sonarr `v3`. Rendering `answered on
 * v1` is a version, not a path, and the value the user has to recognise is the
 * one in their own reverse-proxy config: `/api/v1`. A value that already looks
 * like a path is left alone rather than double-prefixed.
 */
export function apiPathOf(apiVersion: string | undefined): string {
	const value = apiVersion?.trim() ?? '';
	if (value === '') return '';
	if (value.startsWith('/')) return value;
	return `/api/${value}`;
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

/**
 * Whether a row belongs in a "what is wrong" list.
 *
 * FACTORED OUT BECAUSE TWO SCREENS ASK IT. The Services roll-up (§17.3) and
 * Home's Block B (§17.2) are the same question rendered twice, and two copies
 * of this predicate is how one screen reports three problems while the other
 * reports four. The user's only available reading of that is that one of them
 * is lying.
 *
 * `ok` is out because a healthy row is not a problem. A `none` row is out only
 * while the instance is enabled AND has been probed: `turned off` is a fact
 * the owner should see on Home, and so is `not checked yet` on an instance
 * nothing has contacted since it was added.
 */
export function needsAttention(row: ServiceRow, now: Date): boolean {
	const state = stateLabel(row, now);
	if (state.tone === 'ok') return false;
	if (state.tone === 'none' && row.health.enabled && !row.health.stale) return false;
	return true;
}

export function rollup(rows: ServiceRow[], now: Date): RollupEntry[] {
	const out: RollupEntry[] = [];
	for (const row of rows) {
		if (!needsAttention(row, now)) continue;
		const state = stateLabel(row, now);
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

/**
 * `1 error, 2 warnings`, or '' when there is nothing to count.
 *
 * Typed on the one field it reads rather than on `RollupEntry`, so Home's
 * Block B counts its rows with this function instead of a second
 * implementation that pluralises differently.
 */
export function rollupCount(entries: readonly { tone: Tone }[]): string {
	const errors = entries.filter((e) => e.tone === 'err').length;
	const warnings = entries.length - errors;
	const parts: string[] = [];
	if (errors > 0) parts.push(errors === 1 ? '1 error' : `${errors} errors`);
	if (warnings > 0) parts.push(warnings === 1 ? '1 warning' : `${warnings} warnings`);
	return parts.join(', ');
}

// ── Run full sync now (ARCHITECTURE §17.3, http-api.md §4) ─────────────────
//
// The vocabulary for the per-instance catalogue re-import. It is here rather
// than inside a `{#if}` in the template for the reason the header of this file
// gives: the rules below are contract, not taste, and a rule that lives only in
// a template is a rule nothing can test.

/**
 * Where one row's re-import press has got to.
 *
 * ⚠️ THERE IS NO `done`, AND THAT IS THE POINT. The endpoint answers 202 and
 * never learns how the import ended (http-api.md §4.3), so no phase reachable
 * from a response may claim completion. `started` is the strongest word the
 * wire supports. Completion is observed elsewhere entirely: `last_full_sync_at`
 * moving in the `Last successful sync` column, which is written on success
 * alone.
 */
export type SyncPhase = 'idle' | 'starting' | 'started' | 'running' | 'refused' | 'failed';

/**
 * What the screen says after a press, as data.
 *
 * `title` and `consequence` are UsArr's own words; `message` and `action` are
 * the SERVER's, carried through untouched. The split is §17.3's verbatim rule:
 * the upstream's text is never paraphrased and UsArr's own explanation is never
 * dressed up as something the server said.
 */
export interface SyncNote {
	phase: SyncPhase;
	tone: Tone;
	/** UsArr's plain-language headline. */
	title: string;
	/** The server's own `message`, or '' when there was no server message. */
	message: string;
	/** The server's own `action`, or '' when it named none. */
	action: string;
	/** What this press did, and did not do, to the catalogue. */
	consequence: string;
}

/**
 * Whether this instance can be asked to re-import at all.
 *
 * DECIDED ON `role`, EXACTLY AS `syncCell` AND `itemsCell` ALREADY DECIDE. An
 * indexer has no library to read, which is why the wire has a
 * `not_a_catalogue_source` code at all (internal/httpapi/imports.go:89). The
 * two freshness fields cannot answer this — a Prowlarr reports `null` / `0`
 * exactly like a catalogue source that has never synced — so `role` is the only
 * field that separates them (http-api.md §3.2).
 *
 * A DISABLED CATALOGUE SOURCE STILL RETURNS TRUE, deliberately. `enabled` is a
 * flag the user can flip from this same screen, and the server answers a
 * disabled instance with `service_disabled` and the action that fixes it. A
 * button hidden on that row would leave the user with no sentence at all.
 */
export function canRunFullSync(health: ServiceHealth): boolean {
	return !isIndexer(health);
}

/**
 * The 202. `started`, and never a word stronger.
 *
 * ⚠️ NO PROGRESS, NO PERCENTAGE, NO COMPLETION. The response carries none of
 * the three because nothing has been read when the server writes it, so a
 * progress bar here would be an animation over an invented number. The
 * consequence line says the two true things instead: what to watch for, and
 * that a repeat press is how "is it still running" is asked.
 */
export function syncStarted(started: { name: string }): SyncNote {
	return {
		phase: 'started',
		tone: 'ok',
		title: started.name === '' ? 'A full re-import has started' : `${started.name} is re-importing`,
		message: '',
		action: '',
		consequence:
			'Nothing has been read yet, so there is no progress to show. It is finished when Last successful sync moves, and that column moves on success alone.'
	};
}

/**
 * The sentence §4.4 exists to make sayable, and it is said on every non-2xx
 * except `import_in_progress`. A refusal is decided synchronously, so it is a
 * statement about the catalogue and not merely about the request.
 */
const NO_IMPORT_STARTED = 'No import started for this press, so the catalogue is untouched.';

/**
 * Every refusal, told apart on `ApiError.code` and NEVER on the status.
 *
 * ⚠️ THE THREE 409s HAVE OPPOSITE FIXES: wait, press this on a different
 * service, and turn this service on. A branch on `409` would have to guess
 * which of the three sentences to show, and would show the wrong one two times
 * in three (http-api.md §4.4).
 *
 * ⚠️ `import_in_progress` IS NOT A FAILURE and is not toned as one. The work
 * the caller asked for is in flight; the honest word is "already running". It
 * is the state the button is unavailable against, which is why it gets its own
 * phase rather than joining `refused`.
 *
 * Every other code means NO IMPORT STARTED FOR THIS PRESS, because the kind
 * check, the enabled check and the in-progress claim all complete before
 * anything upstream is touched. Only the 500 is a fault; a 409 or a 501 is the
 * server declining, correctly, to do something it cannot do.
 */
export function syncRefusal(
	status: number,
	code: string,
	message: string,
	action: string
): SyncNote {
	const base = { message, action };
	switch (code) {
		case 'import_in_progress':
			return {
				...base,
				phase: 'running',
				tone: 'warn',
				title: 'An import is already running',
				consequence:
					'This press started nothing and disturbed nothing. The import that was already running carries on.'
			};
		case 'not_a_catalogue_source':
			return {
				...base,
				phase: 'refused',
				tone: 'warn',
				title: 'This service has no catalogue to import',
				consequence: NO_IMPORT_STARTED
			};
		case 'service_disabled':
			return {
				...base,
				phase: 'refused',
				tone: 'warn',
				title: 'This service is turned off',
				consequence: NO_IMPORT_STARTED
			};
		case 'not_found':
			return {
				...base,
				phase: 'refused',
				tone: 'warn',
				title: 'This service is no longer there',
				consequence: `${NO_IMPORT_STARTED} Reload the page to see what is configured now.`
			};
		case 'not_configured':
			return {
				...base,
				phase: 'refused',
				tone: 'warn',
				title: 'This build has no catalogue importer wired in',
				consequence: NO_IMPORT_STARTED
			};
		default:
			// The fault branch, and the only one §10's verbatim rule is about: a
			// stored credential that will not open, or anything else the server
			// could not classify. Its own words are rendered, redacted upstream.
			return {
				...base,
				phase: 'failed',
				tone: 'err',
				title: 'The import could not be started',
				consequence:
					status === 0
						? `${NO_IMPORT_STARTED} UsArr could not reach its own server.`
						: NO_IMPORT_STARTED
			};
	}
}

/** The button's label for each phase. `Starting` is the only in-flight word. */
export function syncButtonLabel(phase: SyncPhase): string {
	if (phase === 'starting') return 'Starting';
	if (phase === 'running') return 'Import already running';
	return 'Run full sync now';
}

/** Whether the button may be pressed. §9.3: rendered `aria-disabled`, never `disabled`. */
export function syncButtonBlocked(phase: SyncPhase): boolean {
	return phase === 'starting' || phase === 'running';
}
