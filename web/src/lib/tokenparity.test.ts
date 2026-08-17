/*
 * TOKEN PARITY — docs/design/tokens.css vs web/src/app.css
 *
 * docs/design/tokens.css IS AUTHORITATIVE. web/src/app.css is a HAND-PORT of
 * it: nothing imports tokens.css and nothing generates app.css from it, so a
 * value changed in one arrives in the other only because a human carried it.
 * When this file fails, the default resolution is therefore to fix app.css. The
 * other direction is legitimate only when the design thread has deliberately
 * moved tokens.css first — that is the order, not a tie-break.
 *
 * WHY A TEST AND NOT docs/design/check.mjs. check.mjs §2 does assert token
 * drift, but its file list is tokens.css plus docs/design/mockups/*; app.css is
 * not in it and cannot be seen by it. Probed rather than assumed: poisoning a
 * value in app.css left `node docs/design/check.mjs` green, and the same poison
 * in mockups/usarr.css failed it. check.mjs also runs under `make design`,
 * not `make check`. Vitest runs inside `make check` (Makefile test-web), which
 * is where a gate has to be to hold a commit.
 *
 * WHAT THE GAP COST. It drifted in BOTH directions at once. --status-warn sat
 * stale here for a day after the owner closed the value (app.css kept #8a5300 /
 * #e0a33a against tokens.css's #a44c00 / #fb9349, fixed at 6bac810), while
 * --inset / --hover ran AHEAD of tokens.css, which still records them as raised
 * but not adopted. One direction is not enough; this file checks both.
 *
 * The two files are read with node:fs and parsed as text. Importing them is not
 * an option — app.css is a stylesheet, not a module, and importing it would
 * test whatever the bundler produced rather than what the file says.
 */

import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '../../..');
const TOKENS_CSS = resolve(REPO, 'docs/design/tokens.css');
const APP_CSS = resolve(REPO, 'web/src/app.css');

/* -----------------------------------------------------------------------------
 * The allowlists
 *
 * Three of them — app-only, tokens-only, and same-token-different-value — and
 * every entry is keyed by token name and carries a written reason plus the
 * event that RETIRES it. A blanket "ignore anything app.css has extra" would
 * swallow the next genuine drift, which is the exact failure mode this file
 * exists to close, so nothing here is a category-wide exemption: each is one
 * named token, and the divergence list pins BOTH values so it cannot go stale
 * quietly either.
 *
 * A token declared only in app.css is not automatically wrong — app.css is
 * where a role gets proven before the design thread adopts it. Such entries are
 * a staging area, not a home: each one is eventually routed into tokens.css
 * through the design thread, at which point its line here is deleted and the
 * normal both-directions comparison takes over. Adding one is a single line —
 * e.g. the --font-serif token landing shortly for the wordmark will be app-only
 * until design adopts it, and it belongs here with the same two facts every
 * other entry carries.
 * --------------------------------------------------------------------------- */

/** Declared in app.css, absent from tokens.css. Reason + what retires it. */
const APP_ONLY_ALLOWED: Record<string, string> = {
	'--inset':
		'Ahead of tokens.css deliberately. tokens.css:260-284 records the role and ' +
		'this exact value (#fdfcfb / #100f0e) as raised by the mockup, and says the ' +
		'pass that writes app.css is the one that decides — this is that pass. ' +
		'RETIRED BY: tokens.css declaring --inset in its own ramp.',
	'--hover':
		'Ahead of tokens.css deliberately, same note (#eceae5 / #26241f). A hover ' +
		'fill distinct from --bg-selected, which the §4 mapping cannot express. ' +
		'RETIRED BY: tokens.css declaring --hover in its own ramp.',
	'--bg-inset':
		'The §4 semantic alias for --inset (var(--inset)). It exists here only ' +
		'because --inset does. RETIRED BY: the same adoption that retires --inset.',
	'--scrim':
		'Modal scrim, color-mix over --n-8. A component-layer need that arrived ' +
		'with the mockup port; tokens.css describes roles, and never named this ' +
		'one. RETIRED BY: tokens.css naming a scrim role.',
	'--sidebar-w':
		'Shell geometry, taken from the mockup, which renders at 208px. tokens.css ' +
		'sizes controls rather than the shell, so it never had to name it. ' +
		'RETIRED BY: tokens.css gaining a shell-geometry section.',
	'--font-serif':
		'The Home wordmark, and nothing else in the app, is set in it. tokens.css ' +
		'§4 names two families — sans for the UI, mono for machine data — and a ' +
		'third one arrived here first because the owner settled the serif on the ' +
		'wordmark, not on the palette. app.css is where that gets proven. ' +
		'RETIRED BY: tokens.css declaring --font-serif in its own type section.'
};

/**
 * Declared in BOTH, deliberately at different values. Both values are pinned,
 * so this is not "stop checking this token": if either file moves, the entry
 * stops matching and the drift is reported as usual. That is the difference
 * between a recorded exception and a hole.
 */
const DIVERGENT_ALLOWED: Record<string, { tokens: string; app: string; reason: string }> = {
	'--bg-hover': {
		tokens: 'var(--n-1)',
		app: 'var(--hover)',
		reason:
			'The consequence of adopting --hover early. tokens.css §4 maps hover and ' +
			'raised both to --n-1, which is the mapping it says leaves hover-over-a-' +
			'selected-row with nothing to say; app.css points the alias at the ' +
			'interstitial instead. Same divergence as --hover, one level up. ' +
			'RETIRED BY: the same adoption that retires --hover.'
	}
};

/** Declared in tokens.css, absent from app.css. Reason + what retires it. */
const TOKENS_ONLY_ALLOWED: Record<string, string> = {
	'--spacing':
		"Tailwind v4's base unit, read by Tailwind itself to derive p-3 / gap-2. " +
		'web/ ships no Tailwind — app.css is plain CSS with no @import "tailwindcss" ' +
		'— so porting it would add a token nothing reads. The --space-N scale it ' +
		'anchors IS ported, value for value. RETIRED BY: web/ adopting Tailwind.'
};

/**
 * tokens.css blocks that declare custom properties which are not tokens of the
 * palette. Excluded by prelude, never by token name, so a new block cannot slip
 * past unnoticed — the coverage test below fails on any prelude not listed here
 * and not reachable from a STATE.
 */
const TOKENS_CSS_NON_TOKEN_BLOCKS: Record<string, string> = {
	'@theme inline':
		"Tailwind's theme bridge (--color-bg: var(--bg), --text-xs--line-height, " +
		"…). It re-exports the tokens under Tailwind's namespace rather than " +
		'declaring values, and web/ ships no Tailwind for it to feed. RETIRED BY: ' +
		'web/ adopting Tailwind.'
};

/* -----------------------------------------------------------------------------
 * Parsing
 *
 * Comments are stripped BEFORE anything is matched, so a commented-out
 * declaration cannot be read as a live one. Both files carry the superseded
 * values of --status-warn in prose next to the live ones, which is exactly the
 * text a naive /--status-warn:\s*(\S+)/ would find first.
 *
 * Declarations are collected with their enclosing preludes rather than
 * globally. Every token in both files is declared at least twice — once per
 * theme — so a global match answers a question nobody asked. The third theme
 * block (the prefers-color-scheme media query) is where the last audit found
 * stale values that the first two did not have, and it is only visible to a
 * per-block parse.
 * --------------------------------------------------------------------------- */

interface Decl {
	/** Enclosing preludes, outermost first: at-rule conditions and selectors. */
	readonly preludes: readonly string[];
	readonly name: string;
	/**
	 * The compared form, and the one shown in failures. Canonicalised for the
	 * things the two files differ on WITHOUT differing in meaning: prettier
	 * writes app.css with single quotes and wraps long values across lines,
	 * tokens.css is hand-aligned with double quotes. Comparing the literal text
	 * reports every font stack as drift, which trains the reader to ignore this
	 * file. Showing the canonical form rather than the literal one keeps a
	 * failure legible — two values that differ only in quote style would
	 * otherwise print as an identical-looking pair.
	 */
	readonly value: string;
}

function stripComments(css: string): string {
	// A space, not '', so `--a: 1px/* c */2px` cannot be glued into one token.
	return css.replace(/\/\*[\s\S]*?\*\//g, ' ');
}

function collapse(value: string): string {
	return value.trim().replace(/\s+/g, ' ');
}

function normalise(value: string): string {
	return collapse(value)
		.replace(/'/g, '"') // prettier's quote style in app.css vs tokens.css's
		.replace(/\(\s+/g, '(') // prettier wraps long var()/color-mix() arguments
		.replace(/\s+\)/g, ')')
		.replace(/\s*,\s*/g, ', '); // one space after a comma, both files
}

/**
 * Walks the stylesheet tracking brace depth, and records every custom-property
 * declaration with the preludes it sits inside. Quoted strings are skipped so a
 * brace inside a font stack or a content string cannot desynchronise the depth.
 */
function parse(css: string): Decl[] {
	const out: Decl[] = [];
	const stack: string[] = [];
	let buf = '';

	for (let i = 0; i < css.length; i++) {
		const c = css[i];

		if (c === '"' || c === "'") {
			const end = css.indexOf(c, i + 1);
			const stop = end === -1 ? css.length : end + 1;
			buf += css.slice(i, stop);
			i = stop - 1;
			continue;
		}

		if (c === '{') {
			stack.push(collapse(buf));
			buf = '';
		} else if (c === '}') {
			stack.pop();
			buf = '';
		} else if (c === ';') {
			const m = /^\s*(--[\w-]+)\s*:\s*([\s\S]+)$/.exec(buf);
			if (m) {
				out.push({ preludes: [...stack], name: m[1], value: normalise(m[2]) });
			}
			buf = '';
		} else {
			buf += c;
		}
	}

	return out;
}

/* -----------------------------------------------------------------------------
 * States
 *
 * A token's value is compared as the browser would resolve it in a given state,
 * not as a block-for-block text match. The two files group their selectors
 * differently — app.css writes `:root, :root[data-density='compact'],
 * [data-density='compact']` where tokens.css writes the first two — and a
 * text match on the prelude would call that a drift while missing a real one.
 *
 * The first three states are the three theme blocks, which is the pair this
 * file was written for. The rest exist so that no token tokens.css declares
 * escapes comparison; the coverage test at the bottom enforces exactly that.
 * --------------------------------------------------------------------------- */

interface State {
	readonly label: string;
	readonly attrs: Readonly<Record<string, string | undefined>>;
	readonly prefersDark: boolean;
	readonly reducedMotion: boolean;
}

const STATES: readonly State[] = [
	{ label: ':root — light (no data-theme)', attrs: {}, prefersDark: false, reducedMotion: false },
	{
		label: "[data-theme='dark'] — dark chosen explicitly",
		attrs: { 'data-theme': 'dark' },
		prefersDark: false,
		reducedMotion: false
	},
	{
		label: '@media (prefers-color-scheme: dark) — dark from the OS',
		attrs: {},
		prefersDark: true,
		reducedMotion: false
	},
	{
		label: "[data-density='standard']",
		attrs: { 'data-density': 'standard' },
		prefersDark: false,
		reducedMotion: false
	},
	{
		label: "[data-density='relaxed']",
		attrs: { 'data-density': 'relaxed' },
		prefersDark: false,
		reducedMotion: false
	},
	{
		label: '@media (prefers-reduced-motion: reduce)',
		attrs: {},
		prefersDark: false,
		reducedMotion: true
	}
];

/** null = the prelude is not a media condition this file understands. */
function mediaApplies(prelude: string, state: State): boolean | null {
	if (!prelude.startsWith('@media')) return null;
	const condition = prelude.slice('@media'.length).trim();
	if (condition === '(prefers-color-scheme: dark)') return state.prefersDark;
	if (condition === '(prefers-reduced-motion: reduce)') return state.reducedMotion;
	return false; // e.g. a viewport width — no state models one, so it never applies.
}

/**
 * True when `selector` is root-scoped and its attribute conditions hold in
 * `state`. Anything that is not root-scoped (`.unit--size`) is a component
 * declaring a local variable, not a token, and returns false.
 */
function selectorMatches(selector: string, state: State): boolean {
	let rest = selector.trim();
	if (rest.startsWith(':root')) rest = rest.slice(':root'.length);
	else if (!rest.startsWith('[')) return false;

	const step = /^(?::not\(\[([\w-]+)=(['"])(.*?)\2\]\)|\[([\w-]+)=(['"])(.*?)\5\])/;
	while (rest.length > 0) {
		const m = step.exec(rest);
		if (!m) return false; // an unrecognised combinator or class — not a token block.
		if (m[1] !== undefined) {
			if (state.attrs[m[1]] === m[3]) return false; // :not() excludes this state
		} else if (state.attrs[m[4]] !== m[6]) {
			return false;
		}
		rest = rest.slice(m[0].length);
	}
	return true;
}

function selectorGroupApplies(prelude: string, state: State): boolean {
	return prelude.split(',').some((s) => s.trim().length > 0 && selectorMatches(s, state));
}

function declApplies(decl: Decl, state: State): boolean {
	return decl.preludes.every(
		(prelude) => mediaApplies(prelude, state) ?? selectorGroupApplies(prelude, state)
	);
}

/**
 * The value each token resolves to in `state`. Source order decides, which
 * matches the cascade here because no later block in either file is less
 * specific than an earlier one it overlaps; that is asserted by the density
 * states, which would report the compact values if the assumption broke.
 */
function effective(decls: readonly Decl[], state: State): Map<string, Decl> {
	const out = new Map<string, Decl>();
	for (const decl of decls) if (declApplies(decl, state)) out.set(decl.name, decl);
	return out;
}

const TOKENS_DECLS = parse(stripComments(readFileSync(TOKENS_CSS, 'utf8')));
const APP_DECLS = parse(stripComments(readFileSync(APP_CSS, 'utf8')));

const AUTHORITY = [
	'docs/design/tokens.css is AUTHORITATIVE. web/src/app.css hand-ports it, so',
	'the fix is normally in app.css — unless the design thread moved tokens.css',
	'deliberately, in which case carry the new value across.'
].join('\n');

/**
 * One line per disagreement, naming the token, BOTH values, and which file
 * holds which. "the palettes disagree" is not a usable failure.
 */
function describeDrift(name: string, inTokens: Decl | undefined, inApp: Decl | undefined): string {
	const t = inTokens ? inTokens.value : '(absent)';
	const a = inApp ? inApp.value : '(absent)';
	return `${name}: tokens.css ${t}, app.css ${a}`;
}

/** True only while BOTH sides still hold the values the exception was written for. */
function isRecordedDivergence(name: string, inTokens: Decl, inApp: Decl): boolean {
	const recorded = DIVERGENT_ALLOWED[name];
	return (
		recorded !== undefined && recorded.tokens === inTokens.value && recorded.app === inApp.value
	);
}

describe('web/src/app.css is a faithful hand-port of docs/design/tokens.css', () => {
	for (const state of STATES) {
		it(`carries every tokens.css token, same value, in ${state.label}`, () => {
			const tokens = effective(TOKENS_DECLS, state);
			const app = effective(APP_DECLS, state);
			const drift: string[] = [];

			// tokens.css -> app.css: stale or missing ports.
			for (const [name, decl] of tokens) {
				if (name in TOKENS_ONLY_ALLOWED) continue;
				const ported = app.get(name);
				if (!ported) {
					drift.push(describeDrift(name, decl, undefined));
				} else if (ported.value !== decl.value && !isRecordedDivergence(name, decl, ported)) {
					drift.push(describeDrift(name, decl, ported));
				}
			}

			// app.css -> tokens.css: values invented here, surfaced not ignored.
			for (const [name, decl] of app) {
				if (name in APP_ONLY_ALLOWED) continue;
				if (!tokens.has(name)) drift.push(describeDrift(name, undefined, decl));
			}

			if (drift.length > 0) {
				expect.fail(
					`${drift.length} token(s) disagree in state: ${state.label}\n${AUTHORITY}\n\n` +
						drift.map((line) => `  ${line}`).join('\n')
				);
			}
		});
	}

	it('has a written reason, and a retirement condition, for every allowlisted token', () => {
		const divergent = Object.fromEntries(
			Object.entries(DIVERGENT_ALLOWED).map(([name, entry]) => [name, entry.reason])
		);
		for (const [name, reason] of Object.entries({
			...APP_ONLY_ALLOWED,
			...TOKENS_ONLY_ALLOWED,
			...divergent
		})) {
			expect(reason.length, `${name} needs a reason`).toBeGreaterThan(40);
			expect(reason, `${name} must say what retires it`).toContain('RETIRED BY:');
		}
	});

	it('leaves no allowlist entry standing after the drift it covers is gone', () => {
		const anywhere = (decls: readonly Decl[], name: string) => decls.some((d) => d.name === name);
		for (const name of Object.keys(APP_ONLY_ALLOWED)) {
			expect(
				anywhere(APP_DECLS, name),
				`${name} is allowlisted but app.css no longer declares it`
			).toBe(true);
			expect(
				anywhere(TOKENS_DECLS, name),
				`${name} is now in tokens.css — delete its APP_ONLY_ALLOWED entry`
			).toBe(false);
		}
		for (const name of Object.keys(TOKENS_ONLY_ALLOWED)) {
			expect(
				anywhere(TOKENS_DECLS, name),
				`${name} is allowlisted but tokens.css no longer declares it`
			).toBe(true);
			expect(
				anywhere(APP_DECLS, name),
				`${name} is now in app.css — delete its TOKENS_ONLY_ALLOWED entry`
			).toBe(false);
		}
		for (const name of Object.keys(DIVERGENT_ALLOWED)) {
			// The pinned pair already fails the comparison if either side moves; this
			// catches the entry that outlived the token itself.
			expect(
				anywhere(TOKENS_DECLS, name),
				`${name} is allowlisted but tokens.css no longer declares it`
			).toBe(true);
			expect(
				anywhere(APP_DECLS, name),
				`${name} is allowlisted but app.css no longer declares it`
			).toBe(true);
		}
	});

	// The states above are the whole coverage story, so a tokens.css block that
	// no state reaches is a silent hole rather than a passing test. Fail on it.
	it('reaches every block in tokens.css that declares a token', () => {
		const unreached = new Set<string>();
		for (const decl of TOKENS_DECLS) {
			const key = decl.preludes.join(' >> ');
			if (key in TOKENS_CSS_NON_TOKEN_BLOCKS) continue;
			if (!STATES.some((state) => declApplies(decl, state))) unreached.add(key);
		}
		expect(
			[...unreached],
			'tokens.css declares tokens in a block no STATE reaches. Add the state, or ' +
				'record the block in TOKENS_CSS_NON_TOKEN_BLOCKS with a reason.'
		).toEqual([]);
	});

	it('parsed both files, and did not read a commented-out value as a live one', () => {
		// A regression guard on the parser itself: if either read returns nothing
		// the comparisons above pass vacuously. #8a5300 is the superseded
		// --status-warn, present in BOTH files as prose next to the live value.
		expect(TOKENS_DECLS.length).toBeGreaterThan(100);
		expect(APP_DECLS.length).toBeGreaterThan(100);
		for (const decls of [TOKENS_DECLS, APP_DECLS]) {
			expect(decls.filter((d) => d.name === '--status-warn').map((d) => d.value)).not.toContain(
				'#8a5300'
			);
		}
	});
});
