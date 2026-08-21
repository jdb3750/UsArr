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
import type { ImportProgress, ServiceHealth, ServiceInstance } from './api';

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
	bookorbit: 'BookOrbit',
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
 * ⚠️ NOTHING SHIPPED CALLS THIS, and it is written now because it cannot be
 * retrofitted: the labelling rule is the whole point of the column, and the
 * first DELTA channel must not have to rediscover it.
 *
 * ⚠️ THE REASON GIVEN WAS *"v0.1's only kind is prowlarr, an indexer, so
 * `syncCell()` below answers `Not applicable` for every row that can exist
 * today"*, AND THAT IS DEAD — `internal/httpapi`'s `serviceKinds` carries
 * `kavita` and `bookorbit` at role `library` (ADR-0041, ADR-0052), and
 * `syncCell` answers a real timestamp for both.
 *
 * ⚠️ THE REST OF THAT CORRECTION HAS NOW EXPIRED TOO, and it read *"What is
 * still missing is the DELTA: `internal/libsync` does a full import on connect
 * and there is no change-feed channel behind any adapter, so nothing constructs
 * a `SyncChannel` to pass in here"*. There IS a change-feed channel behind an
 * adapter: BookOrbit's channel 3b, an arrivals-only walk, reachable at
 * `POST /api/v1/services/{id}/sync/delta` (reference/http-api.md §4a). What is
 * still true is the CONSEQUENCE, and only the consequence: nothing constructs a
 * `SyncChannel`, because the server publishes no per-channel sync time for a
 * client to build one FROM — `GET /api/v1/services/health` carries
 * `last_full_sync_at` and no delta equivalent, as the header of this file
 * already says. So this function stays uncalled for a reason about the WIRE, not
 * about the engine.
 *
 * ⚠️ AND THE `kind` UNION IS NOT YET CORRECT FOR THE CHANNEL THAT SHIPPED.
 * `'page-walk delta'` is rendered VERBATIM into `Cell.text`, and it is still the
 * right label for §7.1a's client-side stop — which remains the mechanism for
 * every source that cannot express a since-filter. BookOrbit's 3b is not that
 * shape (ADR-0070), so what it needs is an ADDITIONAL member, not an edit to
 * this one. Choosing the words a user reads is §17.3's decision and not this
 * type's, so the member is deliberately not invented here.
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
 * ⚠️ AND ONE NOTE UNDER ANY OF THEM. When `file_read_failures` is non-zero the
 * cell grows a muted second line saying so; when it is zero there is no second
 * line at all. It is a note about the items' FILE facts, never a state word and
 * never a reason to doubt the count above it — `fileReadNote()` below carries
 * the reasoning.
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
	// The note rides on the SAME cell as the count because it is about these
	// items and about nothing else on the row. It is a `sub` line, which is the
	// muted second line `syncCell` already uses, and never a tone or a state
	// word: http-api.md §3.4 is categorical that a non-zero value leaves the
	// instance healthy.
	const sub = fileReadNote(row.health);
	// A partial import's committed rows are real and are shown, so the count
	// wins over the missing timestamp whenever there is one to show.
	if (workCount <= 0 && lastFullSyncAt === null) return { text: NOTHING.empty, sub, muted: true };
	return { text: workCount.toLocaleString('en-GB'), sub, muted: false };
}

/**
 * The `file_read_failures` note, or '' — and '' is the WHOLE rendering of zero.
 *
 * ⚠️ IT IS A NOTE AND NOT A FAULT. The import completed, the works imported and
 * they are counted in the number above this line; what could not be read is
 * those items' FILE facts (http-api.md §3.4), which is why the availability
 * column elsewhere says "not counted yet" for exactly them. So no tone class, no
 * icon, no `State` word — the muted second line and nothing more. Copy that said
 * the items themselves could not be read would claim they are missing from the
 * library, which is the one thing this number does NOT mean.
 *
 * ⚠️ WHAT COULD NOT BE READ WAS READ FROM THE SERVICE, NOT FROM A DISK. UsArr
 * never touches storage: the walk asks Kavita for each series' volumes
 * (internal/libsync's `StreamFiles`) and a failure is that request going
 * unanswered. `File list` is the thing asked for, so the copy names it rather
 * than naming files on a filesystem UsArr cannot see.
 *
 * ⚠️ ZERO RENDERS NOTHING, DELIBERATELY. `0` on the wire is the positive
 * statement "the last walk read everything it asked for", and a row saying so in
 * words would put a sentence about file reads on every healthy row on the
 * screen. The absence IS the good news; a screen that only speaks when something
 * is worth saying is §17.3's rule for the attention block and the same rule
 * here.
 *
 * The number's window is not this file's to restate — `ServiceHealth.fileReadFailures`
 * in ./api carries it, and it is NOT the window the importer's per-run counter
 * of the same name uses.
 */
function fileReadNote(health: ServiceHealth): string {
	const failures = health.fileReadFailures;
	if (failures <= 0) return '';
	const items = failures === 1 ? 'item' : 'items';
	return `File list not read for ${failures.toLocaleString('en-GB')} ${items}`;
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
 * A 401 or 403 AS AN UPSTREAM ERROR RENDERS ONE, and not as a port number
 * happens to.
 *
 * Every client here formats a status identically — `bookorbit AppInfo: GET
 * /api/v1/app-info -> 403: ...`, and internal/servarr and internal/kavita to the
 * character — so the digits follow a space or a `>`. A bare `401` would also
 * match `dial tcp 10.0.0.4:4013`, and this arm now runs before the transport one,
 * so what precedes the digits is part of the pattern rather than left to luck.
 *
 * 403 is here because it was missing, not merely for symmetry: the password-change
 * refusal reads `-> 403: Password change required` and contains the word
 * "forbidden" nowhere, so it used to fall past the auth arm to the default one and
 * be told the address might not be a BookOrbit at all.
 */
const AUTH_STATUS = /(?:^|[\s>])(?:401|403)\b/;

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
	// ⚠️ THE AUTH ARM RUNS BEFORE THE TRANSPORT ARM, AND THE ORDER IS THE FIX.
	// An arm above another wins every message both would match, and these two
	// overlap on real text: `testBookOrbit` reports an app-info auth failure as
	// "that token was refused on GET /api/v1/app-info" (cmd/usarr/services.go),
	// and the health note it may append carries whatever words the upstream's own
	// indicators used. With the transport arm first, all three of those auth
	// failures were answered with advice about the wrong port.
	//
	// The arms are therefore ordered by STRENGTH OF EVIDENCE: a status code proves
	// the connection completed and the service answered, while a transport word is
	// prose any upstream may repeat back at us.
	//
	// Narrowing the transport arm's `refused` instead was the other candidate, and
	// it is REJECTED for three reasons, in ascending order of weight.
	//
	// It buys nothing. Every transport failure that reaches this function came
	// through the pinned dialer, so it carries net.OpError's own prefix — `ssrf:
	// dial host:port: dial tcp 10.0.0.4:9696: connect: connection refused`
	// (internal/ssrf/dial.go) — and `dial tcp` catches it whatever `refused` does.
	//
	// It fixes one string, not the class. That auth message may carry an appended
	// health note whose text is BookOrbit's own Terminus indicator prose, so an
	// indicator reporting a timeout or an unreachable dependency re-creates the
	// same shadowing through a word narrowing never touched.
	//
	// And it leaves the other half of the defect standing. `-> 403: Password
	// change required` contains no "forbidden" and no "401", so it did not match
	// the auth arm either way; it fell to the default arm and was told the address
	// might not be a BookOrbit. Only matching the status fixes that.
	if (AUTH_STATUS.test(text) || /unauthorized|forbidden|api key/.test(text)) {
		if (kind.trim().toLowerCase() === 'bookorbit') {
			// BookOrbit's credential is a magic-link token, not a key on a
			// settings page, so sending a user to `Settings, General, API Key`
			// would send them somewhere that does not exist.
			//
			// ⚠️ These bullets used to say the token could not be re-read anywhere
			// and to mint a new one. ADR-0060 measured the opposite at
			// bookorbit/bookorbit 73b7877: `magic_access_tokens` stores `raw_token`
			// in plaintext beside `token_hash`, and `MagicLinkRepository.findAll`
			// returns `rawToken` for every row to any superuser behind
			// GET /api/v1/auth/magic-links. So the honest advice is COMPARE, not
			// rotate — minting a replacement destroys the evidence of what went
			// wrong and, when the real fault was a mis-paste, changes nothing.
			//
			// ⚠️ WHAT WOULD MAKE THE FIRST BULLET WRONG: BookOrbit dropping
			// `rawToken` from `findAll`'s select while keeping it in the create
			// response. "A superuser can list every magic link back" would then
			// name a screen that no longer shows the value, and the only honest
			// advice left would be the one ADR-0060 falsified. The behaviour is
			// measured, not promised — re-read it at the commit before trusting
			// this string against a newer BookOrbit.
			//
			// The first bullet also allows for the WRONG TOKEN rather than a bad
			// one, which is a distinct failure with the same 401. It no longer
			// covers a pasted magic-link URL: serviceCredential
			// (internal/httpapi/services.go) strips the token out of one before
			// anything is sent or sealed, because the URL is what BookOrbit's own
			// UI hands out and its own /magic route consumes (ADR-0067). A URL
			// therefore cannot be what is stored, and cannot be what 401s here.
			//
			// The two bullets are the two failures that reach here, in that order:
			// the mint 401, and the 401/403 on the app-info read behind it. The
			// second one used to say "before any call succeeds", which was false of
			// the call that had just succeeded — the login route is @Public() and
			// validates no password, so the mint is exactly the call a pending
			// password change cannot touch. JwtAuthGuard is what refuses, and it
			// only guards the routes after it.
			//
			// It also used to name a CAUSE — a link minted against an ordinary
			// account rather than a shared one — and that half was unfounded:
			// createToken refuses any target that is not shared-provisioned, and
			// no shared account carries isDefaultPassword, so the state cannot be
			// reached that way. See internal/bookorbit's ErrPasswordChangeRequired.
			return [
				`The magic-link token is revoked, expired, deactivated — or it is not the token you think it is. A BookOrbit superuser can list every magic link back, raw value included, and compare it character for character against what is stored here before minting anything new.`,
				`BookOrbit has flagged the account behind the link for a password change, so every guarded route refuses it. Logging in still works, and re-pasting the link will not help: the account state is fixed in BookOrbit.`
			];
		}
		return [
			`The API key is wrong or has been regenerated. It is ${app}'s Settings, General, API Key.`,
			`The key belongs to a different instance. Two ${app} instances have two different keys.`
		];
	}
	if (/refused|no such host|dial tcp|timeout|timed out|deadline|unreachable/.test(text)) {
		return [
			`The wrong port. ${app} listens on its own port and it is not the one the reverse proxy uses.`,
			`The service is not running, or a firewall between this host and it is dropping the connection.`,
			fromHere
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
 * ⚠️ NO PHASE REACHABLE FROM THE RESPONSE MAY CLAIM COMPLETION, and that has
 * not changed. The endpoint answers 202 and never learns how the import ended
 * (http-api.md §4.3), so `started` is still the strongest word any branch of
 * `syncStarted` or `syncRefusal` may use.
 *
 * `finished` IS REACHABLE FROM ONE PLACE AND ONE ONLY: the terminal
 * `import.progress` frame, `phase: "done"`, applied by `syncNoteWithProgress`
 * below. That frame is published after `StampFullSync` has written
 * `last_full_sync_at` (internal/libsync/importer.go), so it is the server
 * saying it succeeded rather than a client concluding it. Silence is not that
 * frame: an import that fails returns before the publish and simply stops
 * sending, so a quiet stream, a dropped socket and a count that reached a total
 * all mean UNKNOWN and none of them may reach this phase.
 *
 * ⚠️ `not_started` AND `stopped` ARE OPPOSITE CLAIMS ABOUT THE CATALOGUE, and
 * they are spelled apart on purpose. `not_started` is the 500 refusal: the
 * press was declined synchronously, nothing upstream was touched, and its own
 * `consequence` says so verbatim. `stopped` is http-api.md §5.5's terminal
 * stream frame: the import DID start, DID run, and stopped partway, and the
 * batches it committed STAND (§5.5.4). §5.5.2 is the section that refuses to
 * let one word carry both, and `not_started` is the replacement it names.
 *
 * ⚠️ `stopped` IS NOT A VERDICT OF FAILURE. The frame carries no cause field
 * and §5.5.5 says it never will carry upstream text, so it genuinely cannot
 * tell a broken upstream from a clean shutdown. It is toned `warn` rather than
 * `err` for exactly that reason: `err` would assert a fault on every
 * cancellation.
 *
 * ⚠️ AND `stopped` IS TERMINAL FOR ONE RUN ONLY. The frame carries
 * `instance_id` and NO run id (§5.5.3), so a later non-terminal frame for that
 * instance is a SECOND RUN starting, not the stop being retracted. That falls
 * out of where this phase is computed rather than needing a rule: it is derived
 * on read from the press note plus the LAST frame, never stored back, so the
 * next `containers` or `items` frame simply re-derives as progress.
 */
export type SyncPhase =
	'idle' | 'starting' | 'started' | 'running' | 'refused' | 'not_started' | 'stopped' | 'finished';

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
 * ⚠️ NO PROGRESS, NO PERCENTAGE, NO COMPLETION IN THIS NOTE. The response
 * carries none of the three because nothing has been read when it is written,
 * so anything numeric here would be invented. Real counts exist, and they reach
 * the screen through `syncNoteWithProgress` folding the `import.progress`
 * frames onto this note afterwards. Until one arrives, the consequence line
 * says the two true things instead: what to watch for, and that a repeat press
 * is how "is it still running" is asked.
 */
export function syncStarted(started: { name: string }): SyncNote {
	return {
		phase: 'started',
		tone: 'ok',
		title: started.name === '' ? 'A full re-import has started' : `${started.name} is re-importing`,
		message: '',
		action: '',
		consequence: NOTHING_READ_YET
	};
}

/**
 * The completion anchor, named because three different sentences end with it.
 *
 * It is the answer that holds when the stream says nothing at all, and it stays
 * true when the stream says plenty: `last_full_sync_at` is written on success
 * alone, so the column moving is positive evidence in every case.
 */
export const WATCH_THE_SYNC_COLUMN =
	'It is finished when Last successful sync moves, and that column moves on success alone.';

/** What the screen said before it could see the stream, kept verbatim as the fallback. */
export const NOTHING_READ_YET = `Nothing has been read yet, so there is no progress to show. ${WATCH_THE_SYNC_COLUMN}`;

/**
 * The sentence §4.4 exists to make sayable, and it is said on every non-2xx
 * except `import_in_progress`. A refusal is decided synchronously, so it is a
 * statement about the catalogue and not merely about the request.
 */
const NO_IMPORT_STARTED = 'No import started for this press, so the catalogue is untouched.';

/**
 * What a stream stop says about the catalogue, and it is the NEGATION of
 * `NO_IMPORT_STARTED` above.
 *
 * ⚠️ NEVER REUSE THE REFUSAL'S SENTENCE HERE. `http-api.md` §5.5.4 names this
 * exactly: "the catalogue is untouched" is FALSE after a stop, because
 * `streamAndApply` commits per batch and every error path returns after the
 * commits that already happened. A stopped import leaves a partially updated
 * catalogue, which is §3.2's real `last_full_sync_at: null` with
 * `work_count > 0` row rather than a contradiction to smooth over.
 */
const STOPPED_ROWS_STAND = 'Applied rows were committed rather than rolled back.';

/**
 * ⚠️ THE FRAME DOES NOT SAY WHY, AND THIS CLIENT MUST NOT GUESS. §5.5.5 ships
 * one phase value and ZERO new fields, deliberately, so there is nowhere for
 * upstream text to land. §5.5.5.1 pins the only shape a cause could ever take
 * if one is added later — a `reason` string from a closed set, each member
 * mapped to a UsArr-authored sentence — and until such a field exists the
 * client says the import stopped and says nothing about why. Inventing a cause
 * from the last phase seen, or from the counters, is how a disk error gets
 * reported to a user as a broken API key.
 */
const STOPPED_WITHOUT_A_CAUSE =
	'The catalogue is not known to be complete, and this frame does not say why the import stopped.';

/**
 * ⚠️ THE STREAM IS THE FAST NOTICE, NOT THE RECORD. §5.5.3: `Hub.Publish` never
 * blocks and drops a subscriber whose queue is full, so this frame is losable.
 * `last_full_sync_at` is written after `rep.Completed = true` and on success
 * alone (§5.5.4), which is what keeps §3's column the authority on whether an
 * import succeeded. So this sentence points AT the column rather than reporting
 * a value for it: the absence of a stop is not evidence of success, and its
 * presence does not override the health read.
 */
const SYNC_COLUMN_ON_A_STOP =
	'Last successful sync does not move on a stop, so that column stays the record of the last import that did succeed.';

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
			// ⚠️ THE LIMITATION IS UsArr's, AND THE TITLE MUST NOT MOVE IT ONTO THE
			// SERVICE. The server's own sentence renders directly beneath this line
			// and reads "UsArr has no catalogue reader for this service"
			// (internal/httpapi/imports.go); a headline saying the service has no
			// catalogue contradicted the line under it.
			//
			// The case that made the contradiction visible has since expired: a
			// BookOrbit registers as `role: library`, so `canRunFullSync` renders the
			// Sync button and the press passed the role gate and was refused deeper,
			// until internal/libsync/bookorbit.go gave it an adapter. The RULE
			// outlives it, and the Go comment now says the same thing: this sentence
			// is shown to whichever kind has no adapter next, and a screen must not
			// tell a user their books do not exist because UsArr cannot read them.
			//
			// No "yet": a Prowlarr reads this same title through the role gate, and
			// there will never be an indexer catalogue to wait for.
			return {
				...base,
				phase: 'refused',
				tone: 'warn',
				title: 'UsArr cannot import from this service',
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
				phase: 'not_started',
				tone: 'err',
				title: 'The import could not be started',
				consequence:
					status === 0
						? `${NO_IMPORT_STARTED} UsArr could not reach its own server.`
						: NO_IMPORT_STARTED
			};
	}
}

/**
 * The count sentence for one live frame, and nothing beyond what the frame said.
 *
 * ⚠️ `total` IS A DENOMINATOR ONLY WHERE IT EXISTS. It is `omitempty` on the
 * wire and only the credits and files passes set one
 * (`internal/libsync/importer.go:531` and `:611`), so the item phase renders a
 * bare count. Rendering `of 0` there, or dividing by it for a percentage, would
 * turn "the source named no total" into "the source said zero", which are
 * opposite claims about how much is left.
 *
 * ⚠️ AND IN BOTH OF THOSE PHASES `applied` IS IN A DIFFERENT UNIT FROM `total`,
 * so neither one may be paired with the other. `credits` reads items and writes
 * credit rows; `files` reads items and writes file rows, and one item carries
 * as many files as it has. Both sentences therefore pair the denominator with
 * the items read and leave the write count standing on its own, which is
 * http-api.md §5.4's rule and is why neither reads as a fraction.
 *
 * ⚠️ AND NO BRANCH HERE SAYS ANYTHING ABOUT THE END. A phase name this client
 * has not heard of still has meaningful counts, so it renders them and declines
 * to name the phase, rather than being dropped or guessed at. That default arm
 * is for a STRANGER, and a phase that has landed upstream is no longer one —
 * `files` took it for as long as it went unnamed here, which api.test.ts's
 * phase-set pin exists to catch.
 */
function progressCounts(progress: ImportProgress): string {
	const read = progress.itemsRead.toLocaleString('en-GB');
	const applied = progress.applied.toLocaleString('en-GB');
	const total = progress.total === undefined ? '' : progress.total.toLocaleString('en-GB');
	switch (progress.phase) {
		case 'containers':
			return 'Reading the list of libraries. Nothing has been applied yet.';
		case 'items':
			return total === ''
				? `Read ${read} items so far, and applied ${applied} of them.`
				: `Read ${read} of ${total} items so far, and applied ${applied} of them.`;
		case 'credits':
			return total === ''
				? `Adding credits: ${read} items read, ${applied} credits written.`
				: `Adding credits: ${read} of ${total} items read, ${applied} credits written.`;
		case 'files':
			return total === ''
				? `Recording files: ${read} items read, ${applied} files recorded.`
				: `Recording files: ${read} of ${total} items read, ${applied} files recorded.`;
		// `applied` is covers FETCHED, and `read` counts every item the pass has
		// settled — including the ones it skipped because the work already had a
		// poster, and the ones the service answered 404 for. So the two legitimately
		// differ by a lot on a re-import, and the wording says "settled" rather than
		// implying every read item produced a cover.
		case 'covers':
			return total === ''
				? `Fetching covers: ${read} items settled, ${applied} covers fetched.`
				: `Fetching covers: ${read} of ${total} items settled, ${applied} covers fetched.`;
		default:
			return `Read ${read} so far, and applied ${applied}.`;
	}
}

/**
 * Fold the live stream into the note the press produced. THE WHOLE FEATURE IS
 * THIS FUNCTION, and its three refusals matter more than its one readout.
 *
 * ⚠️ REFUSAL ONE: NO FRAME, NO CHANGE. `progress` undefined covers a stream
 * that never connected, a stream that connected and has sent nothing yet, and a
 * browser that cannot open one at all. All three get the note exactly as it was
 * written, which is the sentence this screen shipped with. It is still true in
 * every one of them: nothing has been observed, so nothing is claimed.
 *
 * ⚠️ REFUSAL TWO: DISCONNECTED MEANS THE NUMBERS GO AWAY. A socket that dropped
 * mid-import leaves a last frame behind, and that frame is a photograph of a
 * moment that has passed. Rendering it on would be a progress readout that has
 * silently stopped moving, which is worse than none: the user reads a stalled
 * count as a stalled import. So the counts are withdrawn and the sentence says
 * the stream is gone, rather than freezing a number nobody can date.
 *
 * ⚠️ REFUSAL THREE: ONLY A TERMINAL FRAME ENDS IT. Not the last batch, not
 * silence, not `applied` reaching `total`. There are exactly two, they say
 * OPPOSITE things, and neither may be inferred: `done` is success and `stopped`
 * (http-api.md §5.5) is a stop with no cause attached. See SyncPhase.
 *
 * A terminal frame OUTLIVES a disconnection, and that is not an inconsistency
 * with refusal two: it is an event that was observed rather than a count that
 * has to be current, so it stays said whatever the socket does afterwards.
 *
 * ⚠️ AND NEITHER TERMINAL FRAME IS FINAL FOR THE INSTANCE, only for the run.
 * The frame has no run id (§5.5.3), so a later non-terminal frame for the same
 * `instance_id` is a second run starting. This function reads the LAST frame
 * against the press note every time and stores nothing back, so that second run
 * simply renders as progress again, and the stop it replaces was never
 * retracted — it belonged to a run that had already ended.
 *
 * Only a note that is WAITING on an import is folded into. A refusal, a fault
 * and a press still in flight are all statements about the request, and frames
 * for a bootstrap import running in the background must not rewrite them.
 */
export function syncNoteWithProgress(
	note: SyncNote,
	progress: ImportProgress | undefined,
	connected: boolean
): SyncNote {
	if (note.phase !== 'started' && note.phase !== 'running') return note;
	if (progress === undefined) return note;

	if (progress.phase === 'done') {
		const read = progress.itemsRead.toLocaleString('en-GB');
		const applied = progress.applied.toLocaleString('en-GB');
		return {
			...note,
			phase: 'finished',
			tone: 'ok',
			title: 'The import has finished',
			// ⚠️ THE SERVER'S OWN TEXT IS DROPPED HERE, AND ONLY HERE, AND THIS IS
			// NOT §17.3's verbatim rule being bent. That rule forbids PARAPHRASING
			// what the server said; it does not require keeping a sentence about a
			// state that has since ended. The refusal this note may have come from
			// is `import_in_progress`, whose message and action read "an import is
			// already running" and "Wait for the running import to finish" — beside
			// a headline saying it has finished, they are a live instruction to wait
			// for something that is over.
			message: '',
			action: '',
			consequence: `It read ${read} items and applied ${applied}. Last successful sync has moved, because that column is written on success alone.`
		};
	}

	// ⚠️ BEFORE THE `!connected` ARM, EXACTLY AS `done` IS, AND FOR THE SAME
	// REASON. §5.5.3: silence, a socket drop, a reconnect and a `stream.missed`
	// CANNOT downgrade a `stopped` this client has already seen, because silence
	// is UNKNOWN (§5.1) and unknown never overwrites a fact. A stop is an event
	// that was observed, not a count that has to be current, so refusal two's
	// withdrawal of the numbers does not apply to it.
	if (progress.phase === 'stopped') {
		const read = progress.itemsRead.toLocaleString('en-GB');
		const applied = progress.applied.toLocaleString('en-GB');
		return {
			...note,
			phase: 'stopped',
			// `warn`, NOT `err`. See SyncPhase: with no cause field the frame
			// cannot tell a broken upstream from a clean shutdown, and `err`
			// would assert a fault on every cancellation.
			tone: 'warn',
			title: 'The import stopped before it finished',
			// Dropped for the same reason `done` drops them, and it is not
			// §17.3's verbatim rule being bent: the refusal this note may have
			// come from is `import_in_progress`, whose message and action tell
			// the user to wait for a run that has now stopped.
			message: '',
			action: '',
			// §5.5.4's three jobs, in order: say it stopped (the title), say how
			// much stands (`applied`), and say the catalogue is not known to be
			// complete. Never "imported", never a tick, and never a count
			// presented as a result.
			consequence: `It read ${read} items and applied ${applied}. ${STOPPED_ROWS_STAND} ${STOPPED_WITHOUT_A_CAUSE} ${SYNC_COLUMN_ON_A_STOP}`
		};
	}

	if (!connected) {
		return {
			...note,
			consequence: `The progress stream is not connected, so there is no progress to show. ${WATCH_THE_SYNC_COLUMN}`
		};
	}

	return { ...note, consequence: `${progressCounts(progress)} ${WATCH_THE_SYNC_COLUMN}` };
}

/**
 * The button's label for each phase. `Starting` is the only in-flight word.
 *
 * `stopped` is a SETTLED phase and takes the plain label, exactly as
 * `not_started` and `finished` do: the run it describes is over, so pressing
 * again is the correct next move and the button must not be held shut.
 */
export function syncButtonLabel(phase: SyncPhase): string {
	if (phase === 'starting') return 'Starting';
	if (phase === 'running') return 'Import already running';
	return 'Run full sync now';
}

/** Whether the button may be pressed. §9.3: rendered `aria-disabled`, never `disabled`. */
export function syncButtonBlocked(phase: SyncPhase): boolean {
	return phase === 'starting' || phase === 'running';
}
