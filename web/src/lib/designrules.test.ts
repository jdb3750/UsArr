/*
 * §13 OVER web/src — THE STATIC HALF, AND ONLY THE STATIC HALF
 *
 * WHAT THIS FILE ENFORCES. Seven `[grep]` rules from
 * `docs/design/DESIGN-DIRECTION.md` §13, and no others:
 *
 *   1. colour — no indigo / violet / purple / fuchsia
 *   2. colour — no gradient, no bg-clip-text
 *   3. layout — no backdrop-filter / backdrop-blur
 *   4. layout — the border-radius ceiling (6px)
 *   5. type   — no banned family in a font stack
 *   6. type   — no `text-align: center` outside a dialog
 *   7. copy   — no em dash (U+2014) in a string under 15 words
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
 * REASON TWO — THREE §13 CLAUSES ARE ENFORCED BY NEITHER CHECKER. Recorded here
 * so nobody reads a green from either file as coverage of them, and so nobody
 * "fixes" one file to match the other. All three went to the design thread as
 * documentation questions on 2026-08-17:
 *
 *   · §13 bans the four hues, and §13 alone bans a hue by value; see the third
 *     bullet. The centring clause is no longer in this list — design RULED on
 *     it (2026-08-17) rather than leaving it open, and the ruling went §13's
 *     way, so this file's exemption is `dialog|modal` and check.mjs is changing
 *     its side to match. That is a deliberate, ruled divergence from
 *     check.mjs's current regex rather than a mirror, and it is written up at
 *     the rule itself.
 *   · §13 says "Radius tokens: at most three values, maximum 6px." Only the
 *     ceiling has an implementation, in either file. Design ruled on 2026-08-17
 *     that the limit counts NON-ZERO radii and `--radius-0` is the null case,
 *     so `web/src`'s 2px / 4px / 6px satisfies "at most three" and the tree is
 *     compliant; the `0` versus `0px` spelling is an inconsistency, not a hole.
 *     ⚠️ Do not add a count check here — the ceiling is the whole rule.
 *   · §13 bans the four hues "or equivalent hex/oklch". Only the words are
 *     matched, in both files, so a hand-written `#7c3aed` passes today. Design
 *     is defining the ban by OKLCH hue and chroma against a measured floor; it
 *     lands separately, and deliberately not here.
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
