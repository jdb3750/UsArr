/*
 * §13 OVER web/src — THE STATIC HALF, AND ONLY THE STATIC HALF
 *
 * WHAT THIS FILE ENFORCES. Eight `[grep]` rules from
 * `docs/design/DESIGN-DIRECTION.md` §13, and no others:
 *
 *   1. colour — no indigo / violet / purple / fuchsia, BY NAME
 *   2. colour — no indigo / violet / purple / fuchsia, BY VALUE (OKLCH)
 *   3. colour — no gradient, no bg-clip-text
 *   4. layout — no backdrop-filter / backdrop-blur
 *   5. layout — the border-radius ceiling (6px)
 *   6. type   — no banned family in a font stack
 *   7. type   — no `text-align: center` outside a dialog
 *   8. copy   — no em dash (U+2014) in a string under 15 words
 *
 * Rules 1 and 2 are one §13 clause with two halves, and neither half subsumes
 * the other. §13 bans the four families "or equivalent hex/oklch": the word
 * `violet` is caught by 1 and never reaches 2, while `#7c3aed` is caught by 2
 * and never reaches 1. Both ship, and the file says at rule 2 exactly which
 * purples fall between them.
 *
 * ⚠️ ITS GREEN DOES NOT MEAN §13 PASSES, and there are two separate reasons, so
 * nobody should read this file's pass as covering §13 entire.
 *
 * REASON ONE — THE RENDERED RULES ARE NOT HERE AND NEVER WILL BE. Every
 * `[review]` rule in §13 is human judgement. Every RENDERED `[grep]` rule stays
 * in `docs/design/check.mjs`: overflow at five widths, contrast over five
 * grounds, the containment assertion, the roving-tabindex sweep, row heights
 * against the density bands, the accessible names of the availability glyphs,
 * `aria-rowcount` — anything that measures a computed result. The other two copy
 * bans (the banned-word list, and `!` in a UI string) are rendered-corpus rules
 * in check.mjs and are not duplicated here.
 *
 * The dividing rule, stated once:
 *
 *   A static rule can catch a forbidden literal; only a rendered check can
 *   catch a failed outcome.
 *
 * The seven above are decidable from the text of a file: the declaration IS the
 * violation, so reading the source answers the question completely and a browser
 * adds nothing. Moving a rendered rule down here would assert a declaration
 * instead of an effect, which is the `content-visibility`-on-`<tr>` lesson
 * check.mjs records at its check 8 — the declaration was present, the
 * containment was not live, and the guard that read the declaration passed.
 *
 * REASON TWO — TWO §13 CLAUSES ARE ENFORCED BY NEITHER CHECKER. Recorded here
 * so nobody reads a green from either file as coverage of them, and so nobody
 * "fixes" one file to match the other. Both went to the design thread as
 * documentation questions on 2026-08-17:
 *
 *   · §13 bans the four hues, and §13 alone bans a hue by value; rule 2 below
 *     is now that ban, and check.mjs still has no equivalent. The centring
 *     clause is no longer in this list — design RULED on it (2026-08-17) rather
 *     than leaving it open, and the ruling went §13's way, so this file's
 *     exemption is `dialog|modal` and check.mjs is changing its side to match.
 *     That is a deliberate, ruled divergence from check.mjs's current regex
 *     rather than a mirror, and it is written up at the rule itself.
 *   · §13 says "Radius tokens: at most three values, maximum 6px." Only the
 *     ceiling has an implementation, in either file. Design ruled on 2026-08-17
 *     that the limit counts NON-ZERO radii and `--radius-0` is the null case,
 *     so `web/src`'s 2px / 4px / 6px satisfies "at most three" and the tree is
 *     compliant; the `0` versus `0px` spelling is an inconsistency, not a hole.
 *     ⚠️ Do not add a count check here — the ceiling is the whole rule.
 *
 * ⚠️ THE THIRD CLAUSE LEFT THIS LIST ON 2026-08-17, AND IS NOW RULE 2. It read:
 * "§13 bans the four hues 'or equivalent hex/oklch'. Only the words are matched,
 * in both files, so a hand-written `#7c3aed` passes today." It does not pass any
 * more. What has NOT changed is check.mjs's side, which still matches only the
 * words over tokens.css and the mockups, so the two checkers now differ in
 * COVERAGE as well as corpus. That is recorded rather than mirrored: this file
 * does not own check.mjs, and a value ban over the mockups is the design
 * thread's call to make.
 *
 * And one note that is a SUPERSESSION rather than a discrepancy, recorded so a
 * future reader knows which wording won: §13's em-dash bullet cites ARCHITECTURE
 * §17.7, and §13.0 — written later, and describing the mechanism this file
 * copies — reads the whole of §17. §13.0 is the rule. The older §17.7 wording is
 * not a narrower rule the code has drifted from.
 *
 * WHY A VITEST FILE AND NOT check.mjs. check.mjs cannot see `web/src` at all:
 * its sources are `docs/design/tokens.css` and `docs/design/mockups/*`, and it
 * runs under `make design`. Probed rather than assumed, the same way
 * `tokenparity.test.ts` (ddaada9) probed its own gap: poisoning
 * `web/src/app.css` left `node docs/design/check.mjs` green, while the same
 * poison in `mockups/usarr.css` failed it. So the shipped application had never
 * been held to the rules its mockups are held to. Vitest runs inside
 * `make check` (Makefile `test-web`), which is where a gate has to be to hold a
 * commit.
 *
 * THE RULE DEFINITIONS ARE check.mjs's, COPIED, NOT REINTERPRETED. Same regexes,
 * same banned lists, same thresholds, same exemption mechanism, same floors, and
 * the same scanning function. Where §13's prose and check.mjs's code differ,
 * this file encodes check.mjs and reports the difference (see REASON TWO): a
 * third definition that agrees with neither is worse than a documented gap
 * between two. Where the corpus differs — check.mjs reads a rendered DOM for its
 * copy rule and this file reads source text — the difference is written down at
 * the rule that carries it.
 *
 * TWO PROPERTIES INHERITED FROM check.mjs, BECAUSE THEY ARE WHY IT IS TRUSTED:
 *
 *   1. IT ASSERTS WHAT IT LOOKED AT, NOT ONLY WHAT FAILED. A regex that matches
 *      nothing because a glob went stale prints the same green as a genuine
 *      pass, so the corpus and every counted rule declare a floor and fail below
 *      it, and every exception is asserted to have actually matched something.
 *      The floors are derived from today's counts and from what losing one real
 *      file costs, never rounded to a comfortable number.
 *
 *   2. FALSE POSITIVES ARE EXCLUDED STRUCTURALLY, NEVER BY NAME. The first one
 *      is already in the tree: `web/src/routes/+page.svelte` carries the literal
 *      `text-align: center` inside the comment that FORBIDS it ("⚠️ THIS WAS
 *      CENTRED ONCE … DO NOT PUT `text-align: center` BACK"), and
 *      `web/src/app.css` names `backdrop-filter` in the comment that bans it. A
 *      rule cannot fire on its own documentation, so every source is scanned
 *      with comments stripped and line numbers preserved — the same `strip()`
 *      check.mjs uses, widened to Svelte, and the same approach
 *      `tokenparity.test.ts` takes to the superseded token values quoted in
 *      prose beside the live ones. Expect more of them: a rule that forbids a
 *      literal will meet that literal in prose written about the rule.
 *
 * NO EXCEPTION SHIPS. Every rule below is live over the whole of `web/src` with
 * nothing excused by name — which is not how this file started. It found two
 * things on 2026-08-17 and both were RESOLVED rather than parked, which is the
 * outcome an allowlist would have quietly prevented.
 *
 * `web/src/app.css`'s `.th__arrow` carried the one `text-align: center` in the
 * tree: an `aria-hidden` sort glyph in a box one character wide. It was excepted
 * temporarily while design decided, and design decided by MEASUREMENT, the way
 * the wordmark on this same screen was decided. Chromium 141.0.7390.37 against
 * the real build, real ancestry, after `document.fonts.ready`: the box is
 * 7.000 px, `▴` and `▾` both render 5.531 px in IBM Plex Sans 500 at 11px, so
 * centring displaced the glyph 0.734 px. The declaration went. Two reasons worth
 * keeping, because they generalise: an exception has to buy something a reader
 * could notice, and 0.734 px is not that; and the property the fixed box exists
 * to protect — the header not shifting when the sort flips — holds under ANY
 * alignment, because the two arrows measure identically, so the centring was not
 * doing the job it appeared to be doing. It lands opposite to the wordmark
 * deliberately: 60.5 px on a composition there, 0.734 px on a glyph here.
 *
 * NOTHING ELSE IS PARKED. The five short em-dash strings this file found on
 * 2026-08-17 were resolved rather than excepted. Four were reworded by design —
 * three take a colon, because statement-then-reason is what a colon is for, and
 * one takes a full stop, because its second half is an instruction rather than
 * an explanation, which is two acts and therefore two sentences. The fifth was
 * ruled never to have been a violation, and is now excluded by a STRUCTURAL rule
 * rather than by name; see the em-dash test. Design took a finding out of the
 * exercise too: §13's prose endorses the head-dash-explanation construction two
 * paragraphs above banning its punctuation, and §13 is being amended so the
 * endorsement names the colon.
 *
 * ⚠️ AND ONE THING THAT WAS CONSIDERED AND REFUSED, recorded because it will
 * occur to the next person. `.th__arrow`'s centring can be written as
 * `display: inline-flex; justify-content: center`, which does the identical
 * thing and contains none of the banned literal. That is rule-laundering: it
 * would leave this file green while the product did exactly what §13's rule was
 * written about. The real declaration stays, and the exception carries it in the
 * open. If a rule is wrong, argue the rule; do not spell the violation
 * differently.
 */

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '../../..');
const SRC = join(REPO, 'web/src');
const ARCHITECTURE_MD = join(REPO, 'docs/ARCHITECTURE.md');
/* READ-ONLY here, and read for one reason: rule 2's chroma floor is measured off
   it rather than guessed. This file never writes it — tokens.css is the design
   thread's, and `tokenparity.test.ts` owns app.css's agreement with it. */
const TOKENS_CSS = join(REPO, 'docs/design/tokens.css');

/* -----------------------------------------------------------------------------
 * THE CORPUS
 *
 * Everything under `web/src` that ships in the SPA bundle: `.ts`, `.svelte`,
 * `.css`, `.html`.
 *
 * Two exclusions, both structural rather than by name. A `*.test.ts` is not in
 * the bundle — no route imports one and Vite never sees them — and §13's own
 * wording scopes its bans to the shipped surface ("anywhere in the app", "in app
 * CSS", "in the app bundle"). That is also what keeps THIS file out of its own
 * corpus, which matters more than it sounds: the rules below name the literals
 * they ban, so a scan that read its own source would fire on every one of them.
 * `__fixtures__` is recorded upstream data, which is the same exclusion
 * check.mjs makes when it treats a `<td>` as data rather than as the product's
 * own voice.
 * --------------------------------------------------------------------------- */

type Kind = 'ts' | 'svelte' | 'css' | 'html';

interface Source {
	readonly file: string;
	readonly kind: Kind;
	/** Comments blanked, length and line numbers preserved. */
	readonly src: string;
}

function walk(dir: string, out: string[] = []): string[] {
	for (const entry of readdirSync(dir).sort()) {
		const p = join(dir, entry);
		if (statSync(p).isDirectory()) walk(p, out);
		else out.push(p);
	}
	return out;
}

function kindOf(p: string): Kind | null {
	if (p.endsWith('.svelte')) return 'svelte';
	if (p.endsWith('.html')) return 'html';
	if (p.endsWith('.css')) return 'css';
	if (p.endsWith('.ts')) return 'ts';
	return null;
}

/**
 * Blank every comment, keeping the character count and therefore every line
 * number. check.mjs's `strip()`, extended to Svelte, which needs all three kinds
 * at once: HTML comments in the markup, block comments in the `<style>` and the
 * `<script>`, and line comments in the `<script>`.
 *
 * The line-comment arm keeps its guard from check.mjs — a `//` preceded by `:`,
 * a quote or a backslash is a URL or an escape, not a comment — because
 * `href="https://…"` appears in markup this also scans.
 */
function strip(text: string, kind: Kind): string {
	const blank = (m: string) => m.replace(/[^\n]/g, ' ');
	let out = text;
	if (kind === 'svelte' || kind === 'html') out = out.replace(/<!--[\s\S]*?-->/g, blank);
	out = out.replace(/\/\*[\s\S]*?\*\//g, blank);
	if (kind === 'ts' || kind === 'svelte') {
		out = out.replace(/(^|[^:'"\\])\/\/[^\n]*/g, (m, p: string) => p + blank(m.slice(p.length)));
	}
	return out;
}

const FILES: readonly Source[] = walk(SRC)
	.filter((p) => kindOf(p) !== null && !p.endsWith('.test.ts') && !p.includes('__fixtures__'))
	.map((p) => {
		const kind = kindOf(p) as Kind;
		return { file: relative(REPO, p), kind, src: strip(readFileSync(p, 'utf8'), kind) };
	});

/**
 * The least credible size of the scanned corpus, in characters.
 *
 * DERIVED, NOT ROUNDED, which is check.mjs's own test for a floor: one with
 * enough slack to survive the regression it exists to catch is not a floor.
 * Today the stripped corpus is 643,899 characters over 29 files. The two largest
 * are `app.css` (95,077) and `routes/requests/+page.svelte` (94,290), and losing
 * either is exactly the failure this floor is for — a glob that stops matching,
 * a file that moves. 560,000 sits below today's figure by 83,899, which is less
 * than either of them, so losing one fails; and 83,899 characters is far more
 * source than ordinary editing removes.
 */
const CORPUS_FLOOR = 560_000;

function lineOf(text: string, index: number): number {
	return text.slice(0, index).split('\n').length;
}

interface Hit {
	readonly file: string;
	readonly line: number;
	readonly text: string;
	readonly groups: Record<string, string | undefined>;
}

/** check.mjs's `scan()`: every stripped source, one regex, named groups kept. */
function scan(re: RegExp): Hit[] {
	const hits: Hit[] = [];
	for (const f of FILES) {
		const r = new RegExp(re.source, re.flags.includes('g') ? re.flags : re.flags + 'g');
		let m: RegExpExecArray | null;
		while ((m = r.exec(f.src)) !== null) {
			hits.push({
				file: f.file,
				line: lineOf(f.src, m.index),
				text: m[0].trim().slice(0, 120),
				groups: m.groups ?? {}
			});
			if (m.index === r.lastIndex) r.lastIndex++;
		}
	}
	return hits;
}

/** `file:line  text`, which is what a failure has to print to be actionable. */
const fmt = (h: Hit): string => `${h.file}:${h.line}  ${h.text.replace(/\s+/g, ' ')}`;

const collapse = (s: string): string => s.replace(/\s+/g, ' ').trim();

/**
 * §13'S OWN EXEMPTIONS ARE DATA MATCHED AGAINST A NAMED CAPTURE GROUP, never a
 * free-form predicate, and this is check.mjs's mechanism carried over whole
 * rather than re-derived. Its `text-align: center` rule once exempted dialogs by
 * testing the matched DECLARATION — which never contains a selector — so the
 * exemption could not fire under any input, and the defect was invisible
 * because nothing centred existed to exempt. The regex must therefore declare
 * the group by name, and a rule whose pattern lacks it throws here rather than
 * passing vacuously.
 *
 * This is distinct from the NAMED EXCEPTION further down. An exemption here is
 * §13's own carve-out, expressed as a shape; a named exception is one specific
 * thing design has ruled on, and it is spelled out in full.
 */
interface Exemption {
	readonly group: string;
	readonly match: RegExp;
}

function applyRule(re: RegExp, exempt?: Exemption): { hits: Hit[]; exempted: number } {
	if (exempt && !re.source.includes(`(?<${exempt.group}>`)) {
		throw new Error(
			`mis-wired rule: the exemption tests the named group "${exempt.group}", but the ` +
				`pattern has no (?<${exempt.group}>…) group, so it could never fire.`
		);
	}
	const all = scan(re);
	const hits: Hit[] = [];
	let exempted = 0;
	for (const h of all) {
		if (exempt && exempt.match.test(h.groups[exempt.group] ?? '')) exempted++;
		else hits.push(h);
	}
	return { hits, exempted };
}

/* =============================================================================
 * THE COPY CORPUS — user-visible strings, read out of source text
 *
 * This is the one place the corpus genuinely differs from check.mjs, and the
 * difference is forced: check.mjs reads the RENDERED document, walking text
 * nodes and attributes in a real browser, and nothing here has a browser. So the
 * strings are read out of the source instead, from four positions:
 *
 *   · `ts literal`            — a string literal in a `.ts` module
 *   · `svelte script literal` — a string literal in a component's `<script>`
 *   · `attribute`             — a quoted attribute value in markup
 *   · `markup text`           — a run of text between tags
 *
 * TWO STRUCTURAL EXCLUSIONS, both check.mjs's own, applied to the source
 * positions that correspond to them:
 *
 *   · Text and attributes inside a `<td>` are DATA, not copy — a release name, a
 *     timestamp, a publisher called `BOOM! Studios`. check.mjs excludes them by
 *     ancestry in the DOM; here the same exclusion is a tag stack over the
 *     markup, which sees a literal `<td>` and nothing else. A cell rendered by a
 *     component or a snippet is therefore NOT excluded, which is the strict
 *     direction and is left that way deliberately.
 *   · A comment is not copy. Handled once, in `strip()`, for every rule.
 *
 * AND ONE BOUNDARY THAT IS THIS FILE'S ALONE — THE DIVIDING RULE, MET IN
 * PRACTICE. §13's em-dash ban applies to "any string under 15 words". A string
 * built from an interpolation — `` `${event.message} — ${event.action}` ``, or
 * markup reading `{activeService.message}&nbsp;— read at {read.absolute}` — has
 * no word count until it is rendered, because the words are not in the file.
 * Counting the hole as one word invents a short string; counting it as many
 * invents a long one. Neither is a fact about the source. So an interpolated
 * string is not decided here: it is recorded, exactly, in `EMDASH_DEFERRED`, and
 * it belongs to the rendered pass. A static rule can catch a forbidden literal;
 * only a rendered check can catch a failed outcome.
 * ========================================================================== */

interface CopyString {
	readonly source: 'ts literal' | 'svelte script literal' | 'attribute' | 'markup text';
	readonly file: string;
	readonly line: number;
	readonly text: string;
	/** The string contains at least one hole whose words are not in the file. */
	readonly interpolated: boolean;
}

/** U+FFFC OBJECT REPLACEMENT CHARACTER: the standard stand-in for "content here". */
const HOLE = '\uFFFC';

const ENTITIES: Record<string, string> = {
	'&nbsp;': ' ',
	'&amp;': '&',
	'&lt;': '<',
	'&gt;': '>',
	'&quot;': '"',
	'&#39;': "'",
	'&mdash;': '—',
	'&ndash;': '–'
};

/** `&mdash;` is an em dash and must be read as one, or the ban is one entity wide. */
const decode = (s: string): string =>
	s.replace(/&(?:nbsp|amp|lt|gt|quot|#39|mdash|ndash);/g, (m) => ENTITIES[m] ?? m);

/**
 * Elements that do NOT cut a text run.
 *
 * check.mjs's rendered walk cuts a run wherever the nearest blockish ancestor
 * changes, which it can do because it has `getComputedStyle`. Statically the
 * best available proxy is the HTML default: an inline element does not cut, and
 * everything else does. A Svelte COMPONENT (`<Icon />`, `<List />`) is unknown
 * and cuts, which is the strict direction — check.mjs notes that cutting more
 * often makes the em-dash rule stricter rather than laxer, since a short
 * fragment carrying an em dash is precisely what §13 bans.
 */
const INLINE = new Set([
	'a',
	'abbr',
	'b',
	'bdi',
	'bdo',
	'br',
	'cite',
	'code',
	'data',
	'dfn',
	'em',
	'i',
	'kbd',
	'mark',
	'q',
	'rp',
	'rt',
	'ruby',
	's',
	'samp',
	'small',
	'span',
	'strong',
	'sub',
	'sup',
	'time',
	'u',
	'var',
	'wbr'
]);

const VOID = new Set([
	'area',
	'base',
	'br',
	'col',
	'embed',
	'hr',
	'img',
	'input',
	'link',
	'meta',
	'source',
	'track',
	'wbr'
]);

/** String literals — single, double and template — out of a script body. */
function scanScript(
	body: string,
	file: string,
	firstLine: number,
	source: CopyString['source'],
	out: CopyString[]
): void {
	const lit = /'((?:[^'\\\n]|\\.)*)'|"((?:[^"\\\n]|\\.)*)"|`((?:[^`\\]|\\.)*)`/g;
	let m: RegExpExecArray | null;
	while ((m = lit.exec(body)) !== null) {
		let text = m[1] ?? m[2] ?? m[3] ?? '';
		let interpolated = false;
		if (m[3] != null && text.includes('${')) {
			interpolated = true;
			text = text.replace(/\$\{[\s\S]*?\}/g, ` ${HOLE} `);
		}
		text = collapse(text);
		if (text) {
			out.push({ source, file, line: firstLine + lineOf(body, m.index) - 1, text, interpolated });
		}
	}
}

/**
 * The next tag, found without a regex over the whole tag.
 *
 * A Svelte attribute value is an expression in braces and may contain `>`, `<`
 * and quotes — `onclick={(event) => event.preventDefault()}` is the one that
 * broke the first attempt at this, silently welding a handler's source into the
 * button's label and turning a clean §17 string into a violation. So the scan
 * tracks quote and brace depth and ends the tag at the first `>` outside both.
 */
function nextTag(src: string, from: number): { start: number; end: number; raw: string } | null {
	for (let lt = src.indexOf('<', from); lt >= 0; lt = src.indexOf('<', lt + 1)) {
		if (!/[A-Za-z/]/.test(src[lt + 1] ?? '')) continue;
		let depth = 0;
		let quote = '';
		for (let j = lt + 1; j < src.length; j++) {
			const c = src[j];
			if (quote) {
				if (c === quote) quote = '';
			} else if (c === '"' || c === "'") quote = c;
			else if (c === '{') depth++;
			else if (c === '}') depth--;
			else if (c === '>' && depth === 0)
				return { start: lt, end: j + 1, raw: src.slice(lt, j + 1) };
		}
	}
	return null;
}

function scanMarkup(src: string, file: string, out: CopyString[]): void {
	const stack: string[] = [];
	const inData = () => stack.includes('td');

	let run = '';
	let runLine = 1;
	let runInterpolated = false;

	const flush = () => {
		const text = collapse(decode(run));
		run = '';
		const interpolated = runInterpolated;
		runInterpolated = false;
		if (!text || inData()) return;
		out.push({ source: 'markup text', file, line: runLine, text, interpolated });
	};

	const addText = (raw: string, at: number) => {
		/* `{:else}` and `{:else if …}` separate ALTERNATIVES, so joining across one
		   would build a string no reader ever sees — check.mjs hit the same shape
		   with `[hidden]` `[data-inst]` variants concatenated by `textContent`.
		   `{#if}`, `{#each}`, `{/if}` and `{@const}` bound or annotate the same run
		   and are transparent: cutting there would invent a fragment. */
		const parts = raw.split(/\{:[^}]*\}/);
		for (let k = 0; k < parts.length; k++) {
			if (k > 0) flush();
			let part = parts[k].replace(/\{[#/][^}]*\}/g, ' ').replace(/\{@const[^}]*\}/g, ' ');
			if (part.includes('{')) {
				runInterpolated = true;
				part = part.replace(/\{[^{}]*\}/g, ` ${HOLE} `);
			}
			if (!run.trim() && part.trim()) runLine = lineOf(src, at);
			run += part;
		}
	};

	let i = 0;
	for (;;) {
		const tag = nextTag(src, i);
		if (!tag) break;
		addText(src.slice(i, tag.start), i);
		i = tag.end;

		const m = /^<(\/?)([A-Za-z][\w.:-]*)([\s\S]*?)(\/?)>$/.exec(tag.raw);
		if (!m) continue;
		const closing = m[1] === '/';
		const name = m[2].toLowerCase();
		const attrs = m[3] ?? '';
		const selfClosing = m[4] === '/';

		if (!closing && !inData()) {
			const ar = /([A-Za-z_:@][\w:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;
			let a: RegExpExecArray | null;
			while ((a = ar.exec(attrs)) !== null) {
				let value = a[2] ?? a[3] ?? '';
				/* A bare `{expr}` value is a binding, not a string an author wrote. */
				if (/^\s*\{/.test(value)) continue;
				let interpolated = false;
				if (value.includes('{')) {
					interpolated = true;
					value = value.replace(/\{[^{}]*\}/g, ` ${HOLE} `);
				}
				const text = collapse(decode(value));
				if (text) {
					out.push({ source: 'attribute', file, line: lineOf(src, tag.start), text, interpolated });
				}
			}
		}

		if (!INLINE.has(name)) flush();
		if (!VOID.has(name) && !selfClosing) {
			if (closing) {
				const k = stack.lastIndexOf(name);
				if (k >= 0) stack.splice(k, 1);
			} else stack.push(name);
		}
	}
	addText(src.slice(i), i);
	flush();
}

const COPY: readonly CopyString[] = (() => {
	const out: CopyString[] = [];
	const blank = (m: string) => m.replace(/[^\n]/g, ' ');
	for (const f of FILES) {
		if (f.kind === 'ts') {
			scanScript(f.src, f.file, 1, 'ts literal', out);
			continue;
		}
		if (f.kind === 'css') continue; /* see the `content:` assertion below */
		let markup = f.src.replace(
			/<script[^>]*>([\s\S]*?)<\/script>/gi,
			(m, inner: string, off: number) => {
				scanScript(inner, f.file, lineOf(f.src, off), 'svelte script literal', out);
				return blank(m);
			}
		);
		markup = markup.replace(/<style[^>]*>[\s\S]*?<\/style>/gi, blank);
		scanMarkup(markup, f.file, out);
	}
	return out;
})();

/**
 * Per-source floors, each derived from what losing one real file costs.
 *
 * One combined floor would be satisfied by the attribute sweep alone while the
 * markup walk silently contributed nothing, which is the exact shape of the hole
 * check.mjs found when `document.title` sat outside its copy corpus for the
 * whole life of the rule. Today's counts, and the file whose loss each floor
 * catches:
 *
 *   ts literal            717, floor 640 — `lib/api.ts` alone contributes 90
 *   svelte script literal 392, floor 350 — `routes/+layout.svelte` contributes 52
 *   attribute             744, floor 660 — `routes/+page.svelte` contributes 78
 *   markup text           332, floor 295 — `routes/+page.svelte` contributes 36
 */
const COPY_FLOORS: Record<CopyString['source'], number> = {
	'ts literal': 640,
	'svelte script literal': 350,
	attribute: 660,
	'markup text': 295
};

/* -----------------------------------------------------------------------------
 * ARCHITECTURE §17's fixed wording, which is where §13's own em-dash exemption
 * comes from. check.mjs's mechanism, copied whole: the exemption is granted on
 * the em dash's own two-words-either-side window, so a substituted service name
 * or timestamp keeps it and a REWRITTEN phrase loses it, which turns the rule
 * into a copy-drift check between the shipped app and the section that specifies
 * it.
 *
 * ⚠️ §17 EXEMPTS THE APP, AND NOTHING EXEMPTS §17. check.mjs closed that
 * laundering channel by checking §17's own `*"…"*` spans with the exemption
 * withheld, and it still does; this file does not repeat that sweep, because §17
 * is not `web/src` and one corpus should have one owner.
 * --------------------------------------------------------------------------- */

const architecture = readFileSync(ARCHITECTURE_MD, 'utf8');
const s17 = architecture.slice(architecture.indexOf('\n## 17. '));
const s17Body = s17.slice(
	0,
	s17.indexOf('\n## ', 10) === -1 ? undefined : s17.indexOf('\n## ', 10)
);
const norm = (s: string): string =>
	s
		.toLowerCase()
		.replace(/[`*_"“”]/g, '')
		.replace(/\s+/g, ' ')
		.trim();
const fixedBy17 = norm(s17Body);

function exemptedBy17(text: string): boolean {
	const words = norm(text).split(' ');
	for (let i = 0; i < words.length; i++) {
		if (words[i] !== '—') continue;
		const window = words.slice(Math.max(0, i - 2), i + 3).join(' ');
		if (window.split(' ').length >= 3 && fixedBy17.includes(window)) return true;
	}
	return false;
}

/**
 * Em-dash strings whose word count is not a fact about the source.
 *
 * Not exceptions and not findings — a boundary, and the dividing rule met in
 * practice. Each carries an interpolation, so §13's "under 15 words" cannot be
 * evaluated until the string is rendered, and the rendered pass is check.mjs's.
 * Recorded exactly so a new one is a visible change rather than a silent drop:
 * adding an interpolated em-dash string fails this file and asks a human to
 * route it.
 */
const EMDASH_DEFERRED: Record<string, string> = {
	[`web/src/lib/services.ts  no change feed — full compare at ${HOLE}`]:
		"§17.3's sync summary. One hole (a clock), so the rendered string is short — " +
		'but "short" is the rendered fact, not the source one.',
	[`web/src/routes/requests/+page.svelte  ${HOLE} — ${HOLE}`]:
		'`${event.message} — ${event.action}`, twice: the stream-gap notice and the ' +
		'search-failure notice. Both halves come off the wire, so the word count is ' +
		'unknowable here in the strongest sense — it depends on what the server said.',
	[`web/src/routes/requests/+page.svelte  ${HOLE} — read at ${HOLE} , ${HOLE}`]:
		"§17.3's two forms of a timestamp. The head is `{activeService.message}`.",
	[`web/src/routes/requests/+page.svelte  ${HOLE} / ${HOLE} — ${HOLE} seeders, ${HOLE} leechers`]:
		'A visually-hidden expansion of the seeders/leechers cell. Data rather than ' +
		'copy, but rendered by a snippet rather than inside a literal `<td>`, so the ' +
		'tag stack cannot see it.',
	[`web/src/routes/services/+page.svelte  Re-link — this is the same ${HOLE}`]:
		"§17.3's re-identification banner, with the service name interpolated. " +
		'check.mjs exempts the rendered form of this one through §17; the source form ' +
		'has a hole where the two-words-either-side window needs a word.'
};

/* =============================================================================
 * RULE 2'S MACHINERY — §13's colour ban, BY VALUE
 *
 * §13 bans indigo / violet / purple / fuchsia "or equivalent hex/oklch". Rule 1
 * greps the four words, so `#7c3aed` — Tailwind violet-600, and the single most
 * common way a generated UI writes the tell §13 is about — sails straight
 * through it. This closes that by VALUE.
 *
 * A colour trips rule 2 when BOTH hold:
 *
 *   · its OKLCH hue is in [265°, 335°]  — indigo through fuchsia, and
 *   · its OKLCH chroma is >= the floor below.
 *
 * BOTH CONDITIONS ARE LOAD-BEARING AND THE SECOND IS THE ONE PEOPLE DROP. A
 * near-grey at hue 300° is not purple. `#4a464c` is OKLCH hue 314.7° at chroma
 * 0.0112, and so is half of every cool neutral ramp ever shipped — Tailwind's
 * own `zinc-500` is hue 285.9° at chroma 0.0138. A hue-only ban would fail on
 * legitimate neutrals, which is how a colour rule gets deleted rather than
 * fixed. The chroma floor is what separates "purple" from "grey that happens to
 * lean".
 *
 * ─────────────────────────────────────────────────────────────────────────────
 * THE FLOOR IS MEASURED, NOT CHOSEN. Design required this explicitly, and the
 * measurement is reproduced live by the first two rule-2 tests below so it
 * cannot go stale the way a quoted number does.
 *
 * WHAT WAS MEASURED. Every colour value in `docs/design/tokens.css`, in every
 * block — §1 `:root` (light), §2 `:root[data-theme="dark"]`, §3's
 * `prefers-color-scheme: dark` block, and §7's `--shadow-overlay`. 46 values,
 * 0 skipped. Comments are stripped first, so the withdrawn and rejected
 * candidates tokens.css records in prose (`#8a5300`, `#a9700a`, `#e0a33a`, the
 * withdrawn `--protocol-*` pair, CSS `orange` quoted as a hue reference) are
 * correctly NOT counted as tokens.
 *
 * RESULT 1 — THE BAND IS EMPTY. **Zero of the 46 fall in [265°, 335°].** Not
 * one. The ramp is warm by construction (tokens.css: "The ramp is warm-neutral,
 * hue ~35-45") and measures hue 67.6° to 91.6° in OKLCH; the four status values
 * measure 25.8°, 29.0°, 50.6°/53.4° and 151.9°/153.4°. The nearest any token
 * comes to the band is `--n-1` dark `#1e1d1a` at hue 91.6°, which is 173°
 * away. So there is no in-band maximum to clear, and per design's instruction
 * the floor is built on the whole-set maximum instead.
 *
 * RESULT 2 — THE WHOLE-SET MAXIMUM, REGARDLESS OF HUE:
 *
 *       MEASURED MAXIMUM CHROMA   0.179274
 *       (--status-error light, #b3251c, OKLCH L 0.500317 / C 0.179273 / H 28.998°)
 *
 * The constant below is 0.179274 rather than 0.179273 because the exact value is
 * 0.17927323490967592 and a bound the floor must CLEAR is rounded up, not to
 * nearest. That one ulp is not pedantry: it is the difference between the
 * re-measurement test below passing and failing on its own recorded figure.
 *
 * Runners-up, so the maximum is visibly a maximum and not a lone spike:
 * `--status-warn` dark `#fb9349` at 0.152872, `--status-warn` light `#a44c00`
 * at 0.136311, `--status-error` dark `#f0837a` at 0.135056. The whole neutral
 * ramp sits under 0.025 — the most chromatic neutral in either theme is `--n-4`
 * light `#807869` at 0.024371.
 *
 * THE MARGIN, ALSO MEASURED. A floor sitting flush on today's maximum fails the
 * first time a legitimate token is retuned, so the margin is the largest chroma
 * move this palette has ACTUALLY made, taken from tokens.css's own recorded
 * history rather than from a comfortable round number:
 *
 *       --status-warn light, closed by the owner 2026-08-16
 *       #8a5300 (C 0.109252)  ->  #a44c00 (C 0.136311)     delta 0.027058
 *       --status-warn dark, same pass
 *       #e0a33a (C 0.137053)  ->  #fb9349 (C 0.152872)     delta 0.015818
 *
 *       MARGIN = 0.0271, the larger of the two, rounded up.
 *
 * ⚠️ n=2. Those are the only two retunes tokens.css records, so the margin is
 * an honest measurement of a small sample, not a distribution. If a third
 * retune moves a token's chroma by more than 0.0271, re-derive rather than
 * nudging the floor.
 *
 * THE FLOOR:
 *
 *       0.179274 (measured maximum) + 0.0271 (margin) = 0.206374
 *       CHROMA FLOOR = 0.2064, rounded UP, which is the safe direction.
 *
 * BOTH NUMBERS ARE STATED BECAUSE THE MARGIN HAS TO BE VISIBLE, not implied: a
 * reader can see that the floor clears every real token by 0.0271 and clears
 * the entire neutral ramp by more than 0.18.
 *
 * ─────────────────────────────────────────────────────────────────────────────
 * ⚠️ WHAT THIS FLOOR CATCHES, AND WHAT IT LETS THROUGH. A floor forced above the
 * palette's own maximum chroma is a HIGH floor, and honesty about the cost is
 * worth more than a rule that reads as though it catches everything.
 *
 * CAUGHT (chroma >= 0.2064, hue in band) — the saturated purples, which is the
 * generated-UI tell §13 was written about:
 *
 *       #7c3aed  violet-600    C 0.2466  H 293.0°     #9333ea  purple-600   C 0.2525  H 302.3°
 *       #8b5cf6  violet-500    C 0.2189  H 292.7°     #a855f7  purple-500   C 0.2325  H 303.9°
 *       #4f46e5  indigo-600    C 0.2301  H 277.0°     #c026d3  fuchsia-600  C 0.2569  H 322.9°
 *       #8a2be2  blueviolet    C 0.2503  H 301.4°     #d946ef  fuchsia-500  C 0.2591  H 322.2°
 *       #9400d3  darkviolet    C 0.2607  H 309.8°     #ff00ff  CSS fuchsia  C 0.3225  H 328.4°
 *
 * NOT CAUGHT, and each is a real gap rather than a rounding artefact:
 *
 *       #6366f1  indigo-500    C 0.2041  H 277.1°   misses the floor by 0.0023
 *       #800080  CSS purple    C 0.1935  H 328.4°   } caught by rule 1, by name
 *       #4b0082  CSS indigo    C 0.1793  H 301.7°   }
 *       #ee82ee  CSS violet    C 0.1861  H 327.2°   }
 *       #da70d6  CSS orchid    C 0.1813  H 328.7°   caught by NEITHER rule
 *       #c4b5fd  violet-300    C 0.1013  H 293.6°   TINTS ARE NOT CAUGHT AT ALL
 *       #ddd6fe  violet-200    C 0.0549  H 293.3°
 *
 * Three things follow, and they are the reason this list is here rather than
 * summarised away:
 *
 *   1. TINTS ARE OUT OF REACH OF ANY MEASURED FLOOR. A pale violet wash sits at
 *      chroma 0.05-0.10, far below anything this palette uses at any hue, so no
 *      floor derived from the palette can reach it without banning the palette.
 *      That is a limit of the method, accepted deliberately: design ruled that
 *      raising the floor to fit a real token is the failure mode the measurement
 *      exists to prevent, and LOWERING it below the palette's own maximum is the
 *      same error facing the other way.
 *   2. `#4b0082` IS AN ARRESTING COINCIDENCE AND NOTHING MORE. CSS `indigo`
 *      measures chroma 0.17927151; `--status-error` measures 0.17927323. They
 *      differ by 1.7e-6, at hues 272.7° apart — the most saturated colour this
 *      design system uses and the colour §13 is named after are, to five decimal
 *      places, the same distance from grey. Nothing follows from it, but a
 *      reader WILL notice it and should be told it was noticed: it is exactly
 *      why the floor cannot be lowered to catch CSS `indigo` without failing a
 *      real token, and it is the sharpest available illustration of why chroma
 *      alone is not a purple test and the hue band is not optional.
 *   3. RULE 1 AND RULE 2 COMPOSE, AND ONE NAMED COLOUR ESCAPES BOTH. `orchid`,
 *      `plum` and `magenta` are word-banned by neither (rule 1's list is the
 *      four families §13 names) and value-banned by neither (`orchid` is 0.0251
 *      under the floor). Rule 2 does not resolve CSS colour KEYWORDS at all —
 *      no keyword occurs anywhere in `web/src`, verified, and a keyword table is
 *      rule 1's shape of problem, not rule 2's. Recorded as a known gap for the
 *      design thread rather than half-closed here.
 *
 * ─────────────────────────────────────────────────────────────────────────────
 * THE CONVERSION IS IN THIS FILE, FROM THE PUBLISHED MATRICES, AND TAKES NO
 * DEPENDENCY. Björn Ottosson, "A perceptual color space for image processing"
 * (2020-12-23), https://bottosson.github.io/posts/oklab/ — the sRGB gamma
 * decode is CSS Color 4's, https://www.w3.org/TR/css-color-4/#color-conversion-code
 *
 * Validated against the reference value Ottosson publishes: `#ff0000` computes
 * here as L 0.6280 / C 0.2577 / H 29.23°, and `#7c3aed` as L 0.5413 / C 0.2466 /
 * H 293.01°. `srgbToOklch` is asserted against both below, because a conversion
 * nobody checks is a conversion that silently returns grey.
 * ========================================================================== */

interface Oklch {
	/** Perceptual lightness, 0..1. */
	readonly L: number;
	/** Chroma, 0..~0.4 for in-gamut sRGB. */
	readonly C: number;
	/** Hue in degrees, 0..360. */
	readonly H: number;
}

function oklabToOklch(L: number, a: number, b: number): Oklch {
	let H = (Math.atan2(b, a) * 180) / Math.PI;
	if (H < 0) H += 360;
	return { L, C: Math.hypot(a, b), H };
}

/** sRGB channels in 0..1, gamma-encoded, to OKLCH. */
function srgbToOklch(r: number, g: number, b: number): Oklch {
	/* The sRGB transfer function, NOT a 2.2 power law. Eyeballing this as `**2.2`
	   shifts chroma by enough to move a borderline value across the floor, which
	   is exactly the kind of quiet wrongness a measured floor is supposed to
	   exclude. */
	const lin = (c: number): number => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
	const R = lin(r);
	const G = lin(g);
	const B = lin(b);
	const l = Math.cbrt(0.4122214708 * R + 0.5363325363 * G + 0.0514459929 * B);
	const m = Math.cbrt(0.2119034982 * R + 0.6806995451 * G + 0.1073969566 * B);
	const s = Math.cbrt(0.0883024619 * R + 0.2817188376 * G + 0.6299787005 * B);
	return oklabToOklch(
		0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
		1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
		0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s
	);
}

type Parsed =
	{ readonly ok: true; readonly oklch: Oklch } | { readonly ok: false; readonly why: string };

/** A number, or a percentage of `full` — CSS Color 4's reference-range rule. */
function scalar(token: string, full: number): number | null {
	const m = /^([+-]?(?:\d+\.?\d*|\.\d+))(%?)$/.exec(token);
	if (!m) return null;
	return m[2] ? (parseFloat(m[1]) / 100) * full : parseFloat(m[1]);
}

/** An `<angle>`: bare degrees, or deg / grad / rad / turn, normalised to 0..360. */
function angle(token: string): number | null {
	const m = /^([+-]?(?:\d+\.?\d*|\.\d+))(deg|grad|rad|turn)?$/i.exec(token);
	if (!m) return null;
	const v = parseFloat(m[1]);
	const unit = (m[2] ?? 'deg').toLowerCase();
	const deg =
		unit === 'grad'
			? v * 0.9
			: unit === 'rad'
				? (v * 180) / Math.PI
				: unit === 'turn'
					? v * 360
					: v;
	return ((deg % 360) + 360) % 360;
}

/**
 * The three colour components, with the alpha discarded.
 *
 * Legacy `a, b, c` and modern `a b c / alpha` are both live CSS and both occur
 * in this repo's ancestry, so both are read. ALPHA IS DELIBERATELY IGNORED: a
 * violet at 20% opacity is still a violet, and §13 bans the hue, not the
 * coverage. Ignoring it makes the rule stricter, never laxer.
 */
function components(inner: string): string[] {
	return inner
		.split('/')[0]
		.split(/[,\s]+/)
		.map((s) => s.trim())
		.filter(Boolean);
}

function parseHex(text: string): Parsed {
	const s = text.slice(1);
	const w = s.length <= 4 ? 1 : 2;
	const ch = (i: number): number => {
		const d = s.slice(i * w, i * w + w);
		return parseInt(w === 1 ? d + d : d, 16) / 255;
	};
	return { ok: true, oklch: srgbToOklch(ch(0), ch(1), ch(2)) };
}

function parseRgb(inner: string): Parsed {
	const a = components(inner);
	if (a.length !== 3) return { ok: false, why: `rgb() with ${a.length} components, not 3` };
	const ch: number[] = [];
	for (const t of a) {
		const m = /^([+-]?(?:\d+\.?\d*|\.\d+))(%?)$/.exec(t);
		if (!m) return { ok: false, why: `rgb() component "${t}" is not a number or a percentage` };
		ch.push(m[2] ? parseFloat(m[1]) / 100 : parseFloat(m[1]) / 255);
	}
	return { ok: true, oklch: srgbToOklch(ch[0], ch[1], ch[2]) };
}

function parseHsl(inner: string): Parsed {
	const a = components(inner);
	if (a.length !== 3) return { ok: false, why: `hsl() with ${a.length} components, not 3` };
	const h = angle(a[0]);
	const s = scalar(a[1].replace(/%$/, ''), 1);
	const l = scalar(a[2].replace(/%$/, ''), 1);
	if (h === null || s === null || l === null)
		return { ok: false, why: `hsl(${inner}) is not numeric` };
	/* CSS Color 4's reference HSL-to-RGB, §7.1. */
	const S = s / 100;
	const L = l / 100;
	const k = (n: number): number => (n + h / 30) % 12;
	const f = (n: number): number =>
		L - S * Math.min(L, 1 - L) * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1));
	return { ok: true, oklch: srgbToOklch(f(0), f(8), f(4)) };
}

function parseOklch(inner: string): Parsed {
	const a = components(inner);
	if (a.length !== 3) return { ok: false, why: `oklch() with ${a.length} components, not 3` };
	const L = scalar(a[0], 1);
	const C = scalar(a[1], 0.4);
	const H = angle(a[2]);
	if (L === null || C === null || H === null)
		return { ok: false, why: `oklch(${inner}) is not numeric` };
	return { ok: true, oklch: { L, C, H } };
}

function parseOklab(inner: string): Parsed {
	const a = components(inner);
	if (a.length !== 3) return { ok: false, why: `oklab() with ${a.length} components, not 3` };
	const L = scalar(a[0], 1);
	const A = scalar(a[1], 0.4);
	const B = scalar(a[2], 0.4);
	if (L === null || A === null || B === null)
		return { ok: false, why: `oklab(${inner}) is not numeric` };
	return { ok: true, oklch: oklabToOklch(L, A, B) };
}

/**
 * Notations that are colours this file cannot turn into a number, each with the
 * reason it cannot.
 *
 * ⚠️ THESE ARE SKIPPED VISIBLY, NEVER TREATED AS PASSING. Every skip is reported
 * and the whole skip list is asserted against `COLOUR_SKIPS` below, so the first
 * `lab(54% 81 70)` to land in `web/src` fails this file and asks a human where
 * it should go. A rule that silently drops what it cannot read is a rule that
 * passes on the one input that mattered.
 */
const UNCONVERTIBLE: Record<string, string> = {
	'color-mix':
		'a mix of two colours, at least one of which is a var() whose value is not in ' +
		'this file. Statically unresolvable, not merely unimplemented.',
	lab: 'CIE Lab, which needs the D50 white point and an XYZ pipeline this file does not carry',
	lch: 'CIE LCH, same pipeline as lab()',
	color: 'an arbitrary colour space named at the call site (display-p3, rec2020, xyz, …)',
	hwb: 'not implemented: nothing in the corpus writes it, and guessing at an unused notation is worse than skipping it'
};

const CONVERTIBLE: Record<string, (inner: string) => Parsed> = {
	rgb: parseRgb,
	rgba: parseRgb,
	hsl: parseHsl,
	hsla: parseHsl,
	oklch: parseOklch,
	oklab: parseOklab
};

interface ColourValue {
	readonly file: string;
	readonly line: number;
	/** The enclosing CSS selector, where there is one. */
	readonly where: string;
	/** The declaration text leading up to the value, e.g. `--status-error:`. */
	readonly decl: string;
	readonly text: string;
	readonly oklch: Oklch;
}

interface ColourSkip {
	readonly file: string;
	readonly line: number;
	readonly text: string;
	readonly why: string;
}

/**
 * A hex colour is 3, 4, 6 or 8 digits and NOTHING ELSE.
 *
 * Spelling the four lengths out rather than writing `{3,8}` is what keeps a
 * hex-shaped run of some other length from being read as a colour and then
 * reported as an unparseable skip: a 5- or 7-digit run simply does not match.
 *
 * ⚠️ THE TRAILING `\b` IS LOAD-BEARING AND IT COST 21 FALSE POSITIVES TO FIND.
 * This first read `(?![0-9a-fA-F])`, which only refuses a FURTHER HEX DIGIT — so
 * `{#each …}` parsed as `#eac`, the three-digit colour `#eeaacc`, in every
 * Svelte file in the tree. Twenty-one of them, and the rule stayed green purely
 * by luck: `#eeaacc` is OKLCH hue 347.87° at chroma 0.0904, which misses the
 * banned band by 12.87°. A template keyword one letter different would have sat
 * inside it. `\b` refuses any word character, so `#eachXYZ` cannot match while
 * `#faf9f7;` still does.
 *
 * The general lesson, and the reason this is a comment rather than a silent
 * fix: the false positives did not fail anything, they INFLATED THE CORPUS
 * COUNT — 67 values where the tree holds 46. An anti-vacuity floor is measured
 * in the same units a false positive is denominated in, so noise makes a count
 * floor look healthier while making the rule worse. The floor below was derived
 * after this was fixed, never before.
 */
const HEX = /#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{4}|[0-9a-fA-F]{3})\b(?![0-9a-fA-F])/;
/* `oklch|oklab` lead so `lab`/`lch` cannot claim their tails, and `color-mix`
   leads `color` for the same reason. */
const FUNC = /\b(oklch|oklab|rgba?|hsla?|hwb|lch|lab|color-mix|color)\(/i;
const COLOUR_NOTATION = new RegExp(`${HEX.source}|${FUNC.source}`, 'gi');

/** The nearest enclosing `{`'s selector, which is what a reader has to go look at. */
function selectorAt(src: string, index: number): string {
	let depth = 0;
	for (let i = index - 1; i >= 0; i--) {
		const c = src[i];
		if (c === '}') depth++;
		else if (c === '{') {
			if (depth === 0) {
				const from =
					Math.max(
						src.lastIndexOf('}', i - 1),
						src.lastIndexOf('{', i - 1),
						src.lastIndexOf(';', i - 1)
					) + 1;
				return collapse(src.slice(from, i));
			}
			depth--;
		}
	}
	return '';
}

/** What the value is being assigned to: the property, or the line so far. */
function declarationAt(src: string, index: number): string {
	const from =
		Math.max(
			src.lastIndexOf(';', index),
			src.lastIndexOf('{', index),
			src.lastIndexOf('}', index),
			src.lastIndexOf('\n', index)
		) + 1;
	return collapse(src.slice(from, index)).slice(-60);
}

/**
 * Every colour value in one source, converted or visibly skipped.
 *
 * Takes text rather than reading a file, which is what lets the positive control
 * below run THIS extractor over a planted `#7c3aed` instead of a re-implementation
 * of it. A control that exercises a copy of the rule proves nothing about the rule.
 */
function readColours(src: string, file: string, out: ColourValue[], skipped: ColourSkip[]): void {
	const re = new RegExp(COLOUR_NOTATION.source, 'gi');
	let m: RegExpExecArray | null;
	while ((m = re.exec(src)) !== null) {
		const line = lineOf(src, m.index);
		const where = selectorAt(src, m.index);
		const decl = declarationAt(src, m.index);

		if (m[0].startsWith('#')) {
			const parsed = parseHex(m[0]);
			/* Unreachable by construction — HEX only matches valid lengths — but
			   asserted rather than assumed, since a widened regex would otherwise
			   turn into a silent skip. */
			if (!parsed.ok) skipped.push({ file, line, text: m[0], why: parsed.why });
			else out.push({ file, line, where, decl, text: m[0], oklch: parsed.oklch });
			continue;
		}

		/* Balanced parens, because `color-mix(in srgb, var(--n-8) 42%, transparent)`
		   nests one and a non-greedy `\(([^)]*)\)` would cut it at `var(--n-8`. */
		const open = m.index + m[0].length - 1;
		let depth = 0;
		let close = -1;
		for (let i = open; i < src.length; i++) {
			if (src[i] === '(') depth++;
			else if (src[i] === ')' && --depth === 0) {
				close = i;
				break;
			}
		}
		if (close === -1) continue; /* an unterminated call is not a colour value */
		const name = m[1].toLowerCase();
		const inner = src.slice(open + 1, close);
		const text = collapse(src.slice(m.index, close + 1)).slice(0, 80);
		re.lastIndex = close + 1;

		if (name in UNCONVERTIBLE) {
			skipped.push({ file, line, text, why: UNCONVERTIBLE[name] });
			continue;
		}
		const parsed = CONVERTIBLE[name](inner);
		if (!parsed.ok) skipped.push({ file, line, text, why: parsed.why });
		else out.push({ file, line, where, decl, text, oklch: parsed.oklch });
	}
}

/** §13's banned hue band, in OKLCH degrees: indigo through fuchsia. */
const BAND_LO = 265;
const BAND_HI = 335;

/** Measured off tokens.css, and re-measured live below. See the block comment. */
const TOKEN_MAX_CHROMA = 0.179274;
const CHROMA_MARGIN = 0.0271;
const CHROMA_FLOOR = 0.2064;

const inBand = (c: Oklch): boolean => c.H >= BAND_LO && c.H <= BAND_HI;
const banned = (v: ColourValue): boolean => inBand(v.oklch) && v.oklch.C >= CHROMA_FLOOR;

/**
 * The failure line. Names the file, the selector AND the line, and the two
 * computed numbers that decided it, because "banned colour" tells the person
 * reading CI nothing they can act on: they need to know it was the hue that put
 * it in the band and the chroma that cleared the floor.
 */
const reportColour = (v: ColourValue): string =>
	`${v.file}:${v.line}  ${v.where || '(no enclosing selector)'}  ` +
	`${v.decl} ${v.text}  ` +
	`OKLCH hue ${v.oklch.H.toFixed(2)}°, chroma ${v.oklch.C.toFixed(4)}, lightness ${v.oklch.L.toFixed(4)}  ` +
	`(banned band ${BAND_LO}°-${BAND_HI}°, chroma floor ${CHROMA_FLOOR})`;

const CORPUS_COLOURS: ColourValue[] = [];
const CORPUS_COLOUR_SKIPS: ColourSkip[] = [];
for (const f of FILES) readColours(f.src, f.file, CORPUS_COLOURS, CORPUS_COLOUR_SKIPS);

/**
 * Every colour value in `web/src` that rule 2 could not convert, recorded the
 * way `EMDASH_DEFERRED` records its boundary: exactly, so a new one is a visible
 * change rather than a silent pass.
 */
const COLOUR_SKIPS: Record<string, string> = {
	'web/src/app.css  color-mix(in srgb, var(--n-8) 42%, transparent)':
		'--scrim, the modal scrim. `var(--n-8)` is resolved by the browser and not by ' +
		'this file, so the mix has no static value. It is also the one place where ' +
		"skipping costs nothing: --n-8 is the neutral ramp's ink, measured at hue " +
		'78.2° (light) and 87.5° (dark), and a mix towards `transparent` moves alpha ' +
		'rather than hue. tokenparity.test.ts owns this token.'
};

/* =============================================================================
 * THE RULES
 * ========================================================================== */

describe('DESIGN-DIRECTION §13 — the static rules, over web/src', () => {
	it('scans a corpus large enough to be the corpus', () => {
		const chars = FILES.reduce((n, f) => n + f.src.length, 0);
		expect(
			FILES.length,
			'no source files matched under web/src — the walk is looking at the wrong tree'
		).toBeGreaterThanOrEqual(20);
		expect(
			chars,
			`scanned ${chars} characters over ${FILES.length} files, below the floor of ` +
				`${CORPUS_FLOOR}. A check that matches nothing is not a check that passed: ` +
				`something moved, was renamed, or stopped being parsed.`
		).toBeGreaterThanOrEqual(CORPUS_FLOOR);
	});

	it('strips comments, so a rule cannot fire on the prose that documents it', () => {
		/* The positive control for the exclusion every rule below depends on, and it
		   needs no planted fixture: the tree already carries both halves. Asserted in
		   both directions — the comment is really there in the file, and it is really
		   gone from what the rules read — because either assertion alone passes
		   trivially if the other premise has quietly changed. */
		const raw = readFileSync(join(REPO, 'web/src/routes/+page.svelte'), 'utf8');
		const page = FILES.find((f) => f.file === 'web/src/routes/+page.svelte');
		const css = FILES.find((f) => f.file === 'web/src/app.css');
		const html = FILES.find((f) => f.file === 'web/src/app.html');
		expect(page, 'routes/+page.svelte is not in the corpus').toBeDefined();
		expect(css, 'app.css is not in the corpus').toBeDefined();
		expect(html, 'app.html is not in the corpus').toBeDefined();

		expect(raw, 'the comment this control depends on has been edited away').toContain(
			'`text-align: center` BACK'
		);
		expect(page!.src, 'a commented declaration survived stripping').not.toContain(
			'`text-align: center` BACK'
		);
		expect(css!.src, 'a commented backdrop-filter survived stripping').not.toContain(
			'No backdrop-filter'
		);
		expect(html!.src, 'an HTML comment survived stripping').not.toContain('Only the Sans face');
		/* Length-preserving, which is what keeps every reported line number real. */
		expect(page!.src.length, 'stripping changed the file length').toBe(raw.length);
	});

	it('§13 colour: no indigo / violet / purple / fuchsia', () => {
		const { hits } = applyRule(/\b(indigo|violet|purple|fuchsia)\b/i);
		expect(hits.map(fmt), '§13 colour: banned colour family in web/src').toEqual([]);
	});

	it('converts sRGB to OKLCH correctly, against published reference values', () => {
		/* The premise of every number below it. A conversion nobody checks returns
		   grey for everything and reports "no purple found" forever, which is the
		   vacuous-pass shape this file exists to refuse. Both references are
		   Ottosson's own published figures. */
		const of = (hex: string): Oklch => {
			const p = parseHex(hex);
			if (!p.ok) throw new Error(`${hex}: ${p.why}`);
			return p.oklch;
		};

		const red = of('#ff0000');
		expect(red.L).toBeCloseTo(0.628, 3);
		expect(red.C).toBeCloseTo(0.2577, 4);
		expect(red.H).toBeCloseTo(29.23, 2);

		/* Pure white is L 1 at zero chroma, and pure black L 0 — the degenerate
		   cases, which a matrix transcribed with a sign error still gets wrong. */
		expect(of('#ffffff').L).toBeCloseTo(1, 6);
		expect(of('#ffffff').C).toBeCloseTo(0, 6);
		expect(of('#000000').L).toBeCloseTo(0, 6);

		/* The value this whole rule was written for. */
		const violet600 = of('#7c3aed');
		expect(violet600.L).toBeCloseTo(0.5413, 4);
		expect(violet600.C).toBeCloseTo(0.2466, 4);
		expect(violet600.H).toBeCloseTo(293.01, 2);

		/* Every notation must agree on one colour, or the parsers have drifted
		   apart and only the one the corpus happens to use is really tested. */
		const forms = [
			parseRgb('124 58 237'),
			parseRgb('124, 58, 237'),
			parseRgb('48.63% 22.75% 92.94%'),
			parseHsl('262.1 83.3% 57.8%'),
			parseOklch('0.5413 0.2466 293.01'),
			parseOklch('54.13% 61.65% 293.01deg'),
			parseOklab('0.541337 0.096384 -0.226968')
		];
		for (const f of forms) {
			expect(f.ok, `a notation failed to parse: ${f.ok ? '' : f.why}`).toBe(true);
			if (!f.ok) continue;
			expect(f.oklch.C).toBeCloseTo(0.2466, 2);
			expect(f.oklch.H).toBeCloseTo(293.0, 0);
		}
	});

	it("§13 colour by value: the chroma floor still sits above tokens.css's measured maximum", () => {
		/* THE FLOOR'S PREMISE, RE-MEASURED RATHER THAN QUOTED. A number written into
		   a comment goes stale silently — tokens.css §0 carries a whole note about a
		   restated fact that stayed wrong for months (REVIEW-LOG SD-01). So the
		   measurement runs on every `make check` instead, and if the design thread
		   lands a token more chromatic than TOKEN_MAX_CHROMA, or a token inside the
		   banned band, this fails and says re-derive rather than nudge. */
		const raw = readFileSync(TOKENS_CSS, 'utf8');
		const values: ColourValue[] = [];
		const skips: ColourSkip[] = [];
		readColours(strip(raw, 'css'), 'docs/design/tokens.css', values, skips);

		/* 46 values today over four blocks: §1 light, §2 dark and §3 auto contribute
		   15 each (nine ramp steps, two interstitials, four status), and §7's
		   --shadow-overlay contributes one. Floor 32: losing any one theme block
		   drops the count to 31 and fails, which is the regression a count floor is
		   for. Not rounded to 30, which one lost block would survive. */
		expect(
			values.length,
			`only ${values.length} colour value(s) read out of tokens.css, below the floor of ` +
				`32. The floor below is derived from that file, so a parse that finds nothing ` +
				`would "measure" a maximum chroma of zero and pass this trivially.`
		).toBeGreaterThanOrEqual(32);
		expect(
			skips,
			'a tokens.css colour could not be converted, so the measured maximum is not a ' +
				'maximum over the whole file. Convert it or re-derive the floor by hand.'
		).toEqual([]);

		/* RESULT 1: the band is empty. Stated as an assertion, not as prose, so the
		   day a purple token lands the floor's whole derivation is reconsidered. */
		const band = values.filter((v) => inBand(v.oklch));
		expect(
			band.map(reportColour),
			`a tokens.css token now falls in the banned hue band ${BAND_LO}°-${BAND_HI}°. ` +
				`The floor below was derived on the basis that NONE did, so it is no longer ` +
				`the right floor: take the maximum chroma among the in-band tokens and ` +
				`re-derive from that instead.`
		).toEqual([]);

		/* RESULT 2: the whole-set maximum, which is what the floor was built on. */
		const measured = Math.max(...values.map((v) => v.oklch.C));
		const worst = values.find((v) => v.oklch.C === measured)!;
		expect(
			measured,
			`tokens.css's maximum chroma is now ${measured.toFixed(6)} (${reportColour(worst)}), ` +
				`above the ${TOKEN_MAX_CHROMA} the floor of ${CHROMA_FLOOR} was derived from. ` +
				`Re-derive: floor = measured maximum + margin ${CHROMA_MARGIN}.`
		).toBeLessThanOrEqual(TOKEN_MAX_CHROMA);
		expect(
			measured,
			'tokens.css lost its most chromatic token, so the floor is now loose'
		).toBeGreaterThan(TOKEN_MAX_CHROMA - 0.001);

		/* And the arithmetic that turns the two into the floor, asserted rather than
		   trusted to the comment that states it. */
		expect(
			CHROMA_FLOOR,
			`the floor ${CHROMA_FLOOR} no longer clears the measured maximum ` +
				`${TOKEN_MAX_CHROMA} by the stated margin ${CHROMA_MARGIN}`
		).toBeGreaterThanOrEqual(TOKEN_MAX_CHROMA + CHROMA_MARGIN);
	});

	it('§13 colour by value: fires on a planted #7c3aed, and says where and what', () => {
		/* THE DRILL, MADE PERMANENT. A guard that has never been triggered is
		   indistinguishable from no guard, so the rule is fired deliberately here on
		   the exact value the §13 gap was reported for — and through `readColours`,
		   the real extractor, rather than through a re-implementation of it. */
		const planted = ":root[data-theme='dark'] .toolbar {\n\t--accent: #7c3aed;\n}\n";
		const values: ColourValue[] = [];
		const skips: ColourSkip[] = [];
		readColours(planted, 'web/src/planted.css', values, skips);

		expect(skips, 'the planted value was skipped instead of converted').toEqual([]);
		expect(values.length, 'the planted value was not read as a colour at all').toBe(1);
		expect(banned(values[0]), '#7c3aed did not trip the band, so rule 2 is inert').toBe(true);

		/* An actionable message names all four: the file, the selector, the hue that
		   put it in the band and the chroma that cleared the floor. "banned colour"
		   would tell the person reading CI none of it. */
		const message = reportColour(values[0]);
		expect(message).toContain('web/src/planted.css:2');
		expect(message).toContain(":root[data-theme='dark'] .toolbar");
		expect(message).toContain('--accent:');
		expect(message).toContain('#7c3aed');
		expect(message).toContain('hue 293.01°');
		expect(message).toContain('chroma 0.2466');

		/* And the negative half of the drill: the near-grey the chroma floor exists
		   to protect. Same hue band, nowhere near the floor, must NOT trip. */
		const grey: ColourValue[] = [];
		readColours('.row { color: #4a464c; }', 'web/src/planted.css', grey, []);
		expect(grey.length).toBe(1);
		expect(inBand(grey[0].oklch), '#4a464c should be in the hue band').toBe(true);
		expect(banned(grey[0]), 'a near-grey at hue 314.7° must not read as purple').toBe(false);

		/* THE 21 FALSE POSITIVES, PINNED. `{#each}` is Svelte template syntax and
		   `#eac` is not a colour, but a hex pattern that refuses only a further hex
		   digit reads it as `#eeaacc`. See the `\b` on HEX. Pinned as a control
		   rather than trusted to the regex, because the failure mode was invisible:
		   it never turned anything red, it just quietly tripled the corpus count. */
		const svelte: ColourValue[] = [];
		const svelteSkips: ColourSkip[] = [];
		readColours(
			'{#each rows as row (row.id)}<td>{row.title}</td>{/each}\n{#if x}{:else}{/if}',
			'web/src/planted.svelte',
			svelte,
			svelteSkips
		);
		expect(
			[...svelte.map((v) => v.text), ...svelteSkips.map((s) => s.text)],
			'Svelte block syntax is being read as a colour'
		).toEqual([]);
	});

	it('§13 colour by value: no OKLCH purple in web/src', () => {
		/* Anti-vacuity first: 46 values today, all in `app.css`'s three theme blocks
		   (15 each) plus --shadow-overlay. Floor 32 for the same reason as the
		   tokens.css floor above, and it is the floor that matters most here, because
		   every colour in the corpus lives in ONE file. A rule that stopped reading
		   app.css would otherwise print a clean pass over a tree with no colour in
		   it at all. */
		expect(
			CORPUS_COLOURS.length,
			`only ${CORPUS_COLOURS.length} colour value(s) read across web/src, below the ` +
				`floor of 32 — a check that converts nothing is not a check that passed.`
		).toBeGreaterThanOrEqual(32);

		const seen = [...new Set(CORPUS_COLOUR_SKIPS.map((s) => `${s.file}  ${s.text}`))].sort();
		expect(
			seen,
			'a colour value in web/src could not be converted to OKLCH. It is SKIPPED, not ' +
				'passed: rule 2 has no opinion about it, and neither does anything else. ' +
				'Record it in COLOUR_SKIPS with the reason, or change the value to a notation ' +
				'this file can read.\n' +
				CORPUS_COLOUR_SKIPS.map((s) => `${s.file}:${s.line}  ${s.text}  — ${s.why}`).join('\n')
		).toEqual(Object.keys(COLOUR_SKIPS).sort());

		const hits = CORPUS_COLOURS.filter(banned);
		expect(
			hits.map(reportColour),
			`§13 colour: a value in the banned OKLCH band. §13 bans indigo / violet / ` +
				`purple / fuchsia "or equivalent hex/oklch", and rule 1 only reads the words, ` +
				`so this is the half that catches a literal. The floor is MEASURED off ` +
				`tokens.css (maximum chroma ${TOKEN_MAX_CHROMA} + margin ${CHROMA_MARGIN} = ` +
				`${CHROMA_FLOOR}), so anything failing here is more chromatic than every ` +
				`colour the design system uses at any hue, and sits in the purple band. Do ` +
				`not raise the floor to make this green.`
		).toEqual([]);
	});

	it('§13 colour: no gradients or bg-clip-text', () => {
		const { hits } = applyRule(
			/\b(linear-gradient|radial-gradient|conic-gradient|bg-gradient|bg-clip-text)\b/i
		);
		expect(hits.map(fmt), '§13 colour: gradient or bg-clip-text in web/src').toEqual([]);
	});

	it('§13 layout: no backdrop-filter', () => {
		const { hits } = applyRule(/backdrop-(filter|blur)/i);
		expect(hits.map(fmt), '§13 layout: backdrop-filter in web/src').toEqual([]);
	});

	it('§13 layout: every radius ≤ 6px', () => {
		/* check.mjs's pattern, and its history is why it looks like this: a version
		   that read only `border-radius: Npx` matched ZERO in a tree that writes
		   `border-radius: var(--radius-sm)` everywhere, and printed a pass on every
		   run. The ceiling lives in the token DEFINITIONS, so the pattern reads both,
		   and the floor makes a return to zero a failure rather than a pass.

		   ⚠️ THE CEILING ONLY. §13 also says "at most three values", which has no
		   implementation in either checker and which this tree would fail; see REASON
		   TWO in the header. Do not add one here without the design thread. */
		const values = scan(/(?:border-radius|--radius[a-z0-9-]*)\s*:\s*([0-9.]+)px/gi);
		expect(
			values.length,
			`only ${values.length} radius value(s) found, below the floor of 3 — the ` +
				`--radius-* definitions have moved or stopped being parsed`
		).toBeGreaterThanOrEqual(3);
		const over = values.filter((h) => parseFloat(/([0-9.]+)px/.exec(h.text)![1]) > 6);
		expect(over.map(fmt), '§13 layout: border-radius above the 6px ceiling').toEqual([]);
	});

	it('§13 type: no banned family in any font stack', () => {
		/* A font ban is a FONT-STACK ban, evaluated over the comma-separated family
		   list of a `font-family` / `--font-*` declaration and matched as a WHOLE
		   family name. That is what keeps "Internally" and "The Zone of Interest"
		   out: prose containing the substring is not a font stack and is not
		   reachable from here. Structural exclusion, not a list of excused words. */
		const BANNED = ['inter', 'geist', 'space grotesk', 'instrument serif', 'poppins'];
		const decl = /(?:font-family|--font-[a-z-]+|--fs-family)\s*:\s*([^;{}]+)/gi;
		let stacks = 0;
		const bad: string[] = [];
		for (const f of FILES) {
			const r = new RegExp(decl.source, decl.flags);
			let m: RegExpExecArray | null;
			while ((m = r.exec(f.src)) !== null) {
				stacks++;
				for (const family of m[1].split(',')) {
					const name = family
						.trim()
						.replace(/^["']|["']$/g, '')
						.toLowerCase();
					if (BANNED.includes(name)) bad.push(`${f.file}:${lineOf(f.src, m.index)}  ${name}`);
				}
			}
		}
		expect(
			stacks,
			`only ${stacks} font-family declaration(s) parsed, below the floor of 6 — the ` +
				`declarations have moved or stopped being parsed, and a parser that finds ` +
				`nothing reports "no banned family"`
		).toBeGreaterThanOrEqual(6);
		expect(bad, '§13 type: banned family in a font stack').toEqual([]);
	});

	it('§13 type: no text-align:center outside a dialog', () => {
		/* check.mjs's CENTER pattern verbatim. The `where` group is the mechanism,
		   not decoration: the exemption is about WHERE the declaration sits, so the
		   pattern captures the selector (CSS) or the element and its attributes (an
		   inline style) and hands THAT to the exemption. */
		const CENTER =
			/(?<where>[^{}]{0,200}\{[^{}]{0,400}?|<[a-z][^<>]{0,300}?)text-align\s*:\s*center/i;
		/* ⚠️ `dialog|modal`, NOT check.mjs's `dialog|modal|toast` — a deliberate,
		   ruled divergence rather than a copying slip, and the only one in this
		   file. §13 names dialog components and nothing else; design ruled on
		   2026-08-17 that §13 governs here and is changing check.mjs's side to
		   match. The reasoning is worth keeping because it generalises: an unused
		   exemption costs nothing to remove, and silently grants everything the
		   day someone builds the component it names. `toast` matched nothing in
		   either tree — it was a carve-out waiting for its first customer. */
		const { hits } = applyRule(CENTER, { group: 'where', match: /dialog|modal/i });
		/* Reported as `file  selector` rather than `file:line`, because the selector
		   is what a reader has to go and look at. There is no allowlist here and no
		   exception to key one on: `.th__arrow` was the tree's only hit and its
		   declaration was deleted rather than excused. */
		const key = (h: Hit): string => {
			const where = h.groups.where ?? '';
			const brace = where.lastIndexOf('{');
			return `${h.file}  ${collapse(brace === -1 ? where : where.slice(0, brace))}`;
		};
		expect(
			hits.map(key),
			'§13 type: text-align:center outside a dialog.\n' + hits.map(fmt).join('\n')
		).toEqual([]);
	});

	it('reads a copy corpus big enough to be the corpus', () => {
		for (const [source, floor] of Object.entries(COPY_FLOORS)) {
			const n = COPY.filter((s) => s.source === source).length;
			expect(
				n,
				`${n} ${source} string(s) read, below the floor of ${floor} — that source ` +
					`has stopped being collected and is contributing nothing`
			).toBeGreaterThanOrEqual(floor);
		}
	});

	it('§13 copy: no em dash in a string under 15 words', () => {
		const found: string[] = [];
		const deferred: string[] = [];
		const detail: string[] = [];
		let exempted = 0;
		for (const s of COPY) {
			if (!s.text.includes('—')) continue;
			/* STRUCTURAL, AND IT IS THE WHOLE OF THE EXEMPTION: a string whose entire
			   trimmed content is one em dash is the GLYPH, not a sentence with an em
			   dash in it. §13's ban is on the punctuation mark inside copy — "a
			   sentence long enough to need one is already too long for a button" —
			   and there is no sentence here to be too long. Nothing can launder
			   through it, because no sentence hides inside a lone dash.

			   It exists because the rendered checker never needed it: `NOTHING.empty`
			   renders in a `<td>`, and check.mjs excludes a `<td>` as data rather
			   than the product's voice. The static corpus cannot see where a string
			   lands — the constant is defined four files from any cell — so the rule
			   has to be told what the string IS instead of where it goes. Design
			   ruled on 2026-08-17 that this was never a violation, and preferred a
			   structural line here to a named exception that would sit parked. */
			if (s.text === '—') continue;
			if (s.interpolated) {
				deferred.push(`${s.file}  ${s.text}`);
				continue;
			}
			if (s.text.split(/\s+/).length >= 15) continue;
			if (exemptedBy17(s.text)) {
				exempted++;
				continue;
			}
			found.push(`${s.file}  ${s.text}`);
			detail.push(`${s.source} ${s.file}:${s.line}  ${s.text}`);
		}

		/* The §17 exemption absorbing nothing would mean it had silently stopped
		   working, which is how a copy-drift check turns into no check at all. */
		expect(
			fixedBy17.length,
			'ARCHITECTURE §17 could not be located, so the fixed-wording exemption is not ' +
				'being applied at all'
		).toBeGreaterThan(5000);
		expect(
			exempted,
			'the ARCHITECTURE §17 exemption absorbed nothing — either §17 moved, or a ' +
				'label that used to quote it has drifted and should now be failing'
		).toBeGreaterThanOrEqual(1);

		/* No named exceptions here, by design: the four strings this rule found on
		   2026-08-17 were reworded rather than excused, and the fifth is excluded
		   structurally above. An allowlist would have been the easy landing and the
		   worse one — it parks the finding and the copy never changes. */
		expect(found, '§13 copy: em dash in a string under 15 words.\n' + detail.join('\n')).toEqual(
			[]
		);

		const seen = [...new Set(deferred)].sort();
		expect(
			seen,
			'§13 copy: an interpolated em-dash string whose word count is not a fact about ' +
				'the source. A static rule can catch a forbidden literal; only a rendered ' +
				'check can catch a failed outcome — so a new one is recorded in ' +
				'EMDASH_DEFERRED and routed to the rendered pass, never dropped.'
		).toEqual(Object.keys(EMDASH_DEFERRED).sort());
	});

	it('§13 copy: no CSS `content:` string, which this corpus does not read', () => {
		/* Generated content is user-visible copy that check.mjs reads off the rendered
		   DOM and this file's four source positions do not cover. There is none in
		   web/src today, so rather than adding a fifth position with a floor of zero —
		   a floor that cannot fail — the absence is asserted. The first `content: '…'`
		   to land fails here and is told where to go. */
		const strings = scan(/\bcontent\s*:\s*(['"])(?:(?!\1)[^\n])*\1/gi).filter(
			(h) => !/^content\s*:\s*(['"])\1$/.test(h.text)
		);
		expect(
			strings.map(fmt),
			'a CSS `content:` string is user-visible copy, and the copy corpus above reads ' +
				'only TS literals, Svelte script literals, attributes and markup text. Extend ' +
				'the corpus to cover it, or move the string into markup.'
		).toEqual([]);
	});
});
