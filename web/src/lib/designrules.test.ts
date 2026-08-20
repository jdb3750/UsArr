/*
 * §13 AND §17.4 OVER web/src — THE STATIC HALF, AND ONLY THE STATIC HALF
 *
 * WHAT THIS FILE ENFORCES. Eight `[grep]` rules from
 * `docs/design/DESIGN-DIRECTION.md` §13, plus one from `docs/ARCHITECTURE.md`
 * §17.4, and no others:
 *
 *   1. colour — no indigo / violet / purple / fuchsia, BY NAME, plus the five
 *               CSS keywords that are those families under another name
 *               (check.mjs's list, taken whole: orchid, plum, magenta,
 *               lavender, thistle)
 *   2. colour — no indigo / violet / purple / fuchsia, BY VALUE (OKLCH)
 *   3. colour — no gradient, no bg-clip-text
 *   4. layout — no backdrop-filter / backdrop-blur
 *   5. layout — the border-radius ceiling (6px)
 *   6. type   — no banned family in a font stack
 *   7. type   — no `text-align: center` outside a dialog
 *   8. copy   — no em dash (U+2014) in a string under 15 words
 *   9. copy   — §17.4 rule 7: Search's own copy never enumerates media types,
 *               and it names the corpus as "your services"
 *
 * ⚠️ RULE 9 IS THE ONE RULE HERE THAT IS NOT §13's, and it is written down that
 * way rather than filed under §13 for tidiness. Its authority is ARCHITECTURE
 * §17.4 rule 7, its corpus is `web/src/routes/search/**` and not the tree, and
 * §17 wins over §13 wherever the two ever touch (CLAUDE.md: §17 is authoritative
 * over DESIGN-DIRECTION). It ships here because this is the only checker that
 * reads `web/src` at all — see WHY A VITEST FILE below — not because it belongs
 * to §13's family. Its own limits are written at the rule.
 *
 * Rules 1 and 2 are one §13 clause with two halves, and neither half subsumes
 * the other. §13 bans the four families "or equivalent hex/oklch": the word
 * `violet` is caught by 1 and never reaches 2, while `#7c3aed` is caught by 2
 * and never reaches 1. Both ship, and the file says at rule 2 exactly which
 * purples fall between them.
 *
 * ⚠️ ITS GREEN DOES NOT MEAN §13 PASSES, and there are two separate reasons, so
 * nobody should read this file's pass as covering §13 entire. Nor does it mean
 * §17 passes: rule 9 is one rule out of one subsection of it.
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
 * The nine above are decidable from the text of a file: the declaration IS the
 * violation, so reading the source answers the question completely and a browser
 * adds nothing. Moving a rendered rule down here would assert a declaration
 * instead of an effect, which is the `content-visibility`-on-`<tr>` lesson
 * check.mjs records at its check 8 — the declaration was present, the
 * containment was not live, and the guard that read the declaration passed.
 *
 * REASON TWO — FIVE THINGS ARE HELD BY NEITHER CHECKER, IN WHOLE OR AT THE
 * EDGES. Recorded here so nobody reads a green from either file as coverage of
 * them, and so nobody "fixes" one file to match the other. The first two went to
 * the design thread as documentation questions on 2026-08-17; the last three are
 * rulings that came back from it the same day.
 *
 *   · §13 bans the four hues, and §13 alone bans a hue by value; rule 2 below
 *     is now that ban, and check.mjs still has no equivalent. The centring
 *     clause is no longer in this list — design RULED on it (2026-08-17) rather
 *     than leaving it open, the ruling went §13's way, and CHECK.MJS HAS SINCE
 *     NARROWED ITS OWN SIDE to `dialog|modal` — its note at that rule dates the
 *     narrowing. There is no divergence here any more, and the agreement is now
 *     ASSERTED rather than described: both halves of the centring rule are
 *     lifted out of check.mjs's text at test time and compared against this
 *     file's, so either side moving fails here. Written up at the rule itself.
 *   · §13 says "Radius tokens: at most three values, maximum 6px." Only the
 *     ceiling has an implementation, in either file. Design ruled on 2026-08-17
 *     that the limit counts NON-ZERO radii and `--radius-0` is the null case,
 *     so `web/src`'s 2px / 4px / 6px satisfies "at most three" and the tree is
 *     compliant; the `0` versus `0px` spelling is an inconsistency, not a hole.
 *     ⚠️ Do not add a count check here — the ceiling is the whole rule.
 *   · THE PALE VIOLET TINTS, WRITTEN AS RAW HEX, ARE STRUCTURALLY UNCATCHABLE BY
 *     CHROMA IN THIS PALETTE, and that is a property of the colours rather than
 *     a threshold anyone has yet to find. Two measurements, either file:
 *
 *         the neutral ramps' maximum chroma   0.0244   (--n-4 light, #807869)
 *         Tailwind violet-100, #ede9fe        0.0284
 *
 *     Four thousandths apart. Nothing sits between them that is a margin rather
 *     than noise: a floor low enough to fail `#ede9fe` is a floor the palette's
 *     own greys start failing, and a false failure on a legitimate grey is how a
 *     colour rule gets deleted instead of fixed. ⚠️ A RETUNE OF THE FLOOR FROM
 *     0.0515 TO 0.025 WAS PROPOSED ON 2026-08-17 AND WITHDRAWN ON THIS
 *     MEASUREMENT; the floor and its margin below are unchanged, and the
 *     derivation stated there — basis plus the palette's largest recorded
 *     retune — is still the live one and is not a leftover. What the withdrawal
 *     replaced it with is this bullet: the gap is documented rather than tuned.
 *     ⚠️ Do not reopen it by moving the number. What escapes is the HEX
 *     SPELLING only. Both tints are caught by NAME, in both trees, because
 *     `violet` is on rule 1's word list — so closing this wants a mechanism that
 *     is not a chroma threshold (a lightness term is the obvious candidate,
 *     since what makes `#ede9fe` a wash rather than a grey is that it sits at
 *     L 0.943), and that is the design thread's instrument to choose.
 *   · §13's em-dash ban is scoped by WORD COUNT, and rule 8 below applies that
 *     count to a whole text run. Design ruled on 2026-08-17 that this is the
 *     rule working rather than the rule broken — the count is what scopes the
 *     ban to microcopy, which is what the clause is about — but that length was
 *     never the property that actually mattered, so the proxy is poor at both
 *     edges and the ban is soft there in BOTH directions. A long empty-state
 *     paragraph escapes with a dash in it; a short label is caught whether or
 *     not the dash is doing any harm. The Search screen's `pagehead__meta` is
 *     the worked example, measured on 2026-08-17: 29 words as a run and 19 as
 *     the sentence carrying the dash, so it was out of reach at BOTH
 *     granularities and a sentence-scoped rule would have passed it too. It was
 *     fixed by hand. ⚠️ DO NOT TUNE THE 15 to close this; design looked at the
 *     number and left it. Closing it wants a different property, not a
 *     different threshold, and that is the design thread's to choose.
 *   · §17.4 rule 7's THIRD clause — "the silence for the rest is correct, so
 *     this rule does not license copy that explains it" — is enforced by
 *     neither checker, and rule 9 below implements the first two clauses only.
 *     A string that explains why an unsupplied media type is silent is not a
 *     forbidden literal: it is a claim, spelled any number of ways, and telling
 *     it apart from the legitimate sentences on the same screen ("nothing in
 *     UsArr answers a search against your own catalogue yet") is judgement
 *     rather than grep. It is a `[review]` clause in every way but its section
 *     number. What rule 9 does buy against it is indirect and partial: the
 *     likeliest form of the explanation names the types it is explaining, and
 *     clause 1 refuses that.
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
 * ⚠️ RULE 9 IS THE EXCEPTION TO THAT PARAGRAPH, BECAUSE THERE IS NOTHING TO
 * COPY. check.mjs has no §17.4-rule-7 rule in any form: its §17 sweep reads §17's
 * own `*"…"*` spans as a shipping-copy corpus, which holds the SPECIFICATION to
 * §13's copy bans, and says nothing about whether the SPA agrees with the
 * specification. Rule 9 is therefore written from §17.4's text rather than
 * transcribed from a sibling implementation, and its wording is read live out of
 * §17.4 rather than retyped here, so §17 stays the single definition and this
 * file stays a check that the app has not drifted from it.
 *
 * RULE 1'S WORD LIST IS check.mjs's, AND IS NOW IN SYNC. Design widened the
 * gate's list past §13's four families to the CSS keywords that are those
 * families under another name — `5879d50` added `orchid`, `plum` and `magenta`,
 * `2895961` added the pale tints `lavender` and `thistle` — and rule 1 was held
 * at four while that list was moving, on the ground that syncing to a moving
 * list means being wrong in between and doing it twice. The list has settled at
 * nine and rule 1 bans nine; the gap that note recorded is closed.
 * ⚠️ The rule that closed it still stands for the next widening: this list is
 * NOT maintained here. Read it off `docs/design/check.mjs` and take the whole
 * list, or take none of it — a partial sync is the one outcome refused, and
 * there is no exception machinery in this file for a word the gate bans.
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
 * A SIXTH WAS FOUND BY HAND ON THE SAME DAY AND THIS FILE COULD NOT SEE IT. The
 * Search screen's `pagehead__meta` read "Not built yet — below is what it is
 * waiting on"; it now takes the colon, for the reason three of the four above
 * did. Rule 8 never had a chance at it and still would not, which is measured
 * rather than assumed and is written up in REASON TWO. It is here as well as
 * there because the two lists answer different questions: that one records the
 * limit, this one records that the limit has already cost a real string.
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
/* The six, imported rather than restated: rule 9's vocabulary is keyed on them,
   so a seventh media type fails this file instead of going unguarded. This is the
   only import here from the app's own code, and it is data, not behaviour. */
import { MEDIA_TYPES, type MediaType } from './library';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '../../..');
const SRC = join(REPO, 'web/src');
const ARCHITECTURE_MD = join(REPO, 'docs/ARCHITECTURE.md');
/* READ-ONLY here, and read for one reason: rule 2's chroma floor is measured off
   it rather than guessed. This file never writes it — tokens.css is the design
   thread's, and `tokenparity.test.ts` owns app.css's agreement with it. */
const TOKENS_CSS = join(REPO, 'docs/design/tokens.css');
/* READ-ONLY here too, and read for a reason of the same family: the centring
   rule's values are LIFTED from this file at test time rather than transcribed
   out of it. check.mjs is the design thread's; nothing here ever writes it. */
const CHECK_MJS = join(REPO, 'docs/design/check.mjs');

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

/**
 * The least credible size of the scanned corpus, in characters.
 *
 * DERIVED, NOT ROUNDED, which is check.mjs's own test for a floor: one with
 * enough slack to survive the regression it exists to catch is not a floor.
 * ⚠️ RE-DERIVED 2026-08-20, because it had stopped being one. It was 560,000,
 * derived when the stripped corpus was 643,899 characters over 29 files — a gap
 * of 83,899, smaller than either of the two largest files, so losing one failed.
 * The corpus has since reached 1,179,473 characters over 42 files, which left
 * that gap at 616,416: the two largest files could BOTH have vanished and the
 * floor would still have passed. A floor that cannot fail is not a floor, and
 * this one silently stopped being one purely by the tree growing past it.
 *
 * ⚠️ AND THAT RE-DERIVATION WAS ITSELF WRONG. It is recorded here as an error
 * rather than quietly restated with fresh figures, because a comment that
 * acquires new numbers looks maintained and gives nobody a reason to check it.
 * It read: *"The two largest are `app.css` (105,273) and `routes/+page.svelte`
 * (104,457). 1,080,000 sits below today's figure by 99,473 — less than either,
 * so losing one fails."* IT NAMED THE WRONG LARGEST FILE. At the very tree it
 * was committed against, `routes/+page.svelte` was already 106,129 characters,
 * not 104,457 — bigger than `app.css`, so `app.css` was not the largest and
 * 99,473 was not the gap the rule needed checking against. Those figures never
 * described their own tree.
 *
 * The floor still fired, but by 881 characters, and 881 is a coincidence rather
 * than a derivation: ONE comment-only documentation pass over `web/src` added
 * 6,231 characters and the drill below went deaf. That is this guard's own
 * warning about itself — *"a floor nobody has watched fail is a number, not a
 * guard"* — happening to it a second time.
 *
 * RE-DERIVED AGAIN 2026-08-20, on the final tree of that same pass, and stated
 * WITH ITS MARGIN, because a reader deciding whether an edit is safe needs that
 * number visible rather than recomputable:
 *
 *     corpus                                    1,191,498  over 42 files
 *     largest   `routes/+page.svelte`             108,216
 *     second    `app.css`                         105,273
 *     floor                                     1,100,000
 *     gap     corpus − floor                       91,498
 *     MARGIN  floor − (corpus − largest)           16,718
 *
 * The GAP is under both file sizes, so losing EITHER of the two largest files
 * fails the floor — the property the rule has always claimed — and 91,498
 * characters is far more source than ordinary editing removes, which is what
 * stops the floor firing on an honest deletion.
 *
 * THE MARGIN IS THE NUMBER THAT GOES STALE, and it is the one the last two
 * derivations left unstated. It is how far the corpus may GROW before this floor
 * can no longer fail. It was 881. It is 16,718. When a change to `web/src` adds
 * more than that, this floor stops being a floor and must be re-derived — and
 * the drill below is what will say so.
 */
const CORPUS_FLOOR = 1_100_000;

/**
 * The least credible NUMBER of files, which is the char floor's blind spot:
 * twenty small components can disappear without moving the character count much.
 *
 * Derived against losing a DIRECTORY, the shape a broken walk actually takes.
 * On 2026-08-20 the walk reads 42 files — 25 under `lib/`, 15 under `routes/`,
 * 2 at the root. Losing `routes/` leaves 27 and losing `lib/` leaves 17, so 30
 * fails on either, and the remaining 12 files of slack is more than ordinary
 * churn moves.
 */
const FILE_FLOOR = 30;

/**
 * ONE PREDICATE, TWO CALLERS, and that is the point: the corpus read consults it
 * before vouching for what it has, and the test below fires it. A floor asserted
 * only in a test is a floor that a filtered run (`vitest -t …`) skips, which is
 * the run someone reaches for when they are chasing one rule.
 *
 * Returns the complaint, or `null` when the corpus is credible.
 */
function corpusShortfall(files: readonly Source[]): string | null {
	if (files.length < FILE_FLOOR) {
		return (
			`${files.length} source file(s) read under web/src, below the floor of ${FILE_FLOOR} — ` +
			`the walk is looking at the wrong tree, or a directory has gone missing from it`
		);
	}
	const chars = files.reduce((n, f) => n + f.src.length, 0);
	if (chars < CORPUS_FLOOR) {
		return (
			`scanned ${chars} characters over ${files.length} files, below the floor of ` +
			`${CORPUS_FLOOR}. A check that matches nothing is not a check that passed: ` +
			`something moved, was renamed, or stopped being parsed.`
		);
	}
	return null;
}

/**
 * THE THREE FILESYSTEM CALLS THE CORPUS READ MAKES, BEHIND AN INTERFACE, so the
 * read's behaviour under a tree that is changing beneath it can be DRILLED
 * rather than asserted. Nothing in the gate substitutes this; only the drills do.
 */
interface CorpusIO {
	readonly list: (dir: string) => string[];
	readonly isDir: (p: string) => boolean;
	readonly read: (p: string) => string;
}

const nodeIO: CorpusIO = {
	list: (dir) => readdirSync(dir).sort(),
	isDir: (p) => statSync(p).isDirectory(),
	read: (p) => readFileSync(p, 'utf8')
};

/** One whole-tree walk-and-read. All of it, or it throws — there is no partial return. */
function collect(io: CorpusIO): Source[] {
	const paths: string[] = [];
	const walk = (dir: string): void => {
		for (const entry of io.list(dir)) {
			const p = join(dir, entry);
			if (io.isDir(p)) walk(p);
			else paths.push(p);
		}
	};
	walk(SRC);
	return paths
		.filter((p) => kindOf(p) !== null && !p.endsWith('.test.ts') && !p.includes('__fixtures__'))
		.map((p) => {
			const kind = kindOf(p) as Kind;
			return { file: relative(REPO, p), kind, src: strip(io.read(p), kind) };
		});
}

/**
 * THE READ IS TAKEN ONCE, AND IT IS NEVER NARROWED. This is the second way this
 * check failed under a shared checkout: `web/src` is walked live, so a merge
 * landing in another lane can unlink or half-write a file between the `readdir`
 * that lists it and the `read` that opens it. The symptom was this check failing
 * outright, on a run whose own diff was three Go files.
 *
 * ⚠️ THERE IS NO RETRY, AND THAT IS THE RULING, NOT AN OVERSIGHT. Design ruled on
 * 2026-08-20 that reading a tree while another process rewrites it is a real
 * concurrency defect in the same family as two agents sharing one checkout — not
 * flakiness — and that "a retry would make the symptom vanish and leave a test
 * that silently reads inconsistent trees". A loop that eventually gets a clean
 * pass has not read a coherent tree; it has read several incoherent ones and
 * kept the one that did not complain.
 *
 * ⚠️ AND NO PER-FILE `catch`, for the same reason arriving by the other door.
 * Skipping the file that threw makes the check pass by reading LESS, and
 * green-because-nothing-was-wrong and green-because-nothing-was-read are then
 * the same colour. So `collect` produces the whole tree or throws, and this
 * function turns either an incomplete walk or a corpus below its floors into one
 * loud failure that says what happened and whose diff it is not about.
 *
 * A STABLE SOURCE WAS CONSIDERED AND REFUSED. Reading from a fixed git ref would
 * be perfectly coherent, and would stop testing the thing the rule is for: this
 * runs in a PRE-COMMIT gate, so the `text-align: center` it exists to catch is
 * by definition not committed yet. A check that never flinches but reads the
 * wrong tree is worse than one that reports the real condition of this one.
 */
function readCorpus(io: CorpusIO = nodeIO): readonly Source[] {
	let files: Source[];
	try {
		files = collect(io);
	} catch (e) {
		throw new Error(
			`web/src could not be read whole, so every §13 rule below would have been scanning ` +
				`a corpus this file cannot vouch for.\n` +
				`A file moved between the readdir that listed it and the read that opened it. ` +
				`That is another process rewriting this checkout while the suite ran — a merge ` +
				`landing in a shared tree — and it is not the diff being gated. It is reported ` +
				`rather than retried: a read that has to be attempted twice has not seen one ` +
				`coherent tree, and a loop around it would only hide that.\n` +
				(e instanceof Error ? e.message : String(e)),
			{ cause: e }
		);
	}
	const short = corpusShortfall(files);
	if (short !== null) {
		throw new Error(
			`web/src was read without error and came back too small to be web/src, which is the ` +
				`failure that would otherwise have gone GREEN — every rule reports a clean sheet ` +
				`over a corpus that is not there.\n${short}`
		);
	}
	return files;
}

const FILES: readonly Source[] = readCorpus();

function lineOf(text: string, index: number): number {
	return text.slice(0, index).split('\n').length;
}

interface Hit {
	readonly file: string;
	readonly line: number;
	readonly text: string;
	readonly groups: Record<string, string | undefined>;
}

/**
 * A MANDATORY-LITERAL PREFILTER, and the reason `scan` takes options at all.
 *
 * `anchor` is a source FRAGMENT of the rule it accompanies, never a rule of its
 * own. `scan` compiles it with the RULE'S flags, so it can never be stricter
 * than the rule is about the same characters, and it refuses an anchor whose
 * source is not literally part of the rule's. When the fragment is a mandatory
 * tail — every alternative has to reach it to match — a file that does not
 * contain the fragment cannot contain a hit, so skipping that file changes no
 * result. The substring refusal is the mechanical half of that argument; the
 * other half is that the pattern must be BUILT from the fragment, which is why
 * `CENTER` below is assembled from `CENTRED` rather than retyped around it.
 *
 * IT EXISTS BECAUSE ONE RULE WAS QUADRATIC AND SILENT ABOUT IT. `CENTER`'s
 * `[^{}]{0,200}\{[^{}]{0,400}?` prefix is retried at every offset of the
 * corpus: measured on 2026-08-20 it cost 855 ms over 1,176,416 characters and
 * returned nothing — 97% of this file's entire run, rising with the corpus, and
 * one loaded machine away from tripping vitest's 5 s per-test DEFAULT and
 * turning red on whoever happened to be gating. The anchor puts it at 1 ms by
 * never starting the expensive scan on a file that cannot match.
 *
 * Fixing the cost rather than the bound is the point. Nothing here asserts an
 * elapsed time and nothing should: `CLAUDE.md` rules that "wall-clock
 * benchmarks belong in `make bench`, never in a merge gate", so vitest's
 * timeout stays what it is meant to be — a guard against a hang — and this
 * check stays far enough under it that the guard never has an opinion about it.
 */
interface ScanOpts {
	/** A source fragment of `re`, compiled with `re`'s own flags. */
	readonly anchor?: RegExp;
	/** The corpus to read. Defaults to the whole of `FILES`. */
	readonly over?: readonly Source[];
}

/** check.mjs's `scan()`: every stripped source, one regex, named groups kept. */
function scan(re: RegExp, opts: ScanOpts = {}): Hit[] {
	let sources: readonly Source[] = opts.over ?? FILES;
	if (opts.anchor) {
		if (!re.source.includes(opts.anchor.source)) {
			throw new Error(
				`mis-wired anchor: /${opts.anchor.source}/ is not part of the rule's own pattern, ` +
					`so prefiltering on it could skip a file that holds a real hit.`
			);
		}
		const pre = new RegExp(opts.anchor.source, re.flags.replace('g', ''));
		sources = sources.filter((f) => pre.test(f.src));
	}
	const hits: Hit[] = [];
	for (const f of sources) {
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

function applyRule(
	re: RegExp,
	exempt?: Exemption,
	opts?: ScanOpts
): { hits: Hit[]; exempted: number } {
	if (exempt && !re.source.includes(`(?<${exempt.group}>`)) {
		throw new Error(
			`mis-wired rule: the exemption tests the named group "${exempt.group}", but the ` +
				`pattern has no (?<${exempt.group}>…) group, so it could never fire.`
		);
	}
	const all = scan(re, opts);
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
 * RULE 9'S MACHINERY — ARCHITECTURE §17.4 rule 7, over the Search screen
 *
 * THE RULE, QUOTED AS WRITTEN, because a paraphrase of it is what produced the
 * string it exists to forbid. `docs/ARCHITECTURE.md` §17.4, rule 7:
 *
 *   "Search's own copy never enumerates media types, and the corpus it names is
 *   *your services*. The set of types behind that phrase is whatever the
 *   connected services supply, which is a different set on a Prowlarr-only
 *   install than on a full stack, so any fixed list is a promise the install may
 *   not keep. **A shorter list is the same defect with fewer words** — the
 *   failure is enumeration, not arity."
 *
 * ⚠️ IT SAYS "SEARCH'S OWN COPY", AND THAT IS THE CORPUS THIS RULE TAKES. The
 * rule is §17.4's, §17.4 is the Search screen, and the scope is neither widened
 * to `web/src` nor narrowed below the route. Widening it was tried and rejected
 * on evidence rather than on principle: run over the whole tree it fails
 * `web/src/lib/requests.ts`, whose indexer-coverage note reads "the trackers that
 * carry ebooks and audiobooks are invite-only". That string is §17.5's copy, it
 * is about which trackers carry what rather than about what the library holds,
 * and rule 7 does not govern it. A tree-wide port would therefore have landed
 * either a false failure or the named exception this file refuses to keep. If
 * design wants the ban on every screen, that is §17's sentence to write and this
 * rule's corpus to widen afterwards.
 *
 * TWO OF THE RULE'S THREE CLAUSES ARE IMPLEMENTED HERE:
 *
 *   1. NO ENUMERATION — a Search copy string naming two or more media types
 *      fails. Two, not three: §17 says a shorter list is the same defect, so the
 *      threshold is the smallest number that is a list at all. One mention is
 *      not an enumeration and is not touched.
 *   2. THE CORPUS IS NAMED — §17's standing wording must still be on the screen,
 *      read live out of §17.4 rather than retyped here. That makes it a
 *      copy-drift check in the same shape as the em-dash rule's §17 exemption:
 *      §17 stays the single definition, and this file only asserts that the app
 *      has not drifted from it.
 *
 * The third clause — the silence for an unsupplied type must not be explained —
 * is judgement rather than grep, and is recorded in the header's REASON TWO
 * alongside the §13 clauses no checker holds. It is NOT implemented below, and
 * nothing here should be read as covering it.
 *
 * WHAT CLAUSE 1 CANNOT SEE, STATED BEFORE THE CODE RATHER THAN AFTER IT:
 *
 *   · IT IS A WORD LIST, WHICH IS RULE 1'S SHAPE OF PROBLEM AND HAS RULE 1'S
 *     HOLE. The six types are derived — `MEDIA_TYPES` is imported from
 *     `$lib/library`, and a seventh fails the coverage test below rather than
 *     going quietly unguarded — but the WORDS for each are hand-written, because
 *     the string this rule was written about ("film, episode, album, book and
 *     comic") uses none of the six enum values. A synonym nobody listed walks
 *     through. There is no derivation available that would fix this: the enum is
 *     the schema's vocabulary and copy is English.
 *   · THREE OBVIOUS WORDS ARE DELIBERATELY OFF THE LIST — `series`, `show` and
 *     `track` — because each is a common non-media word in this product's own
 *     prose ("a series of", "show the user", "keep track of"), and a copy rule
 *     that fires on prose gets deleted rather than fixed. That is the same
 *     structural argument the font rule makes about "Internally". The cost is
 *     real and is bounded: a two-item enumeration built ONLY from excluded words
 *     ("series and shows") passes. A longer list survives losing one word,
 *     because two of the remaining ones still trip it.
 *   · IT READS THE SOURCE, SO IT CANNOT SEE A LIST ASSEMBLED AT RUNTIME. Copy
 *     built by joining `MEDIA_TYPES` through `mediaTypeLabel()` would enumerate
 *     every type on screen and contain no type word in the file at all. That is
 *     the dividing rule again, and it belongs to a rendered pass.
 *   · IT READS THE ROUTE, SO IT CANNOT FOLLOW COPY OUT OF IT. If Search's
 *     strings move into a `$lib` module the way Requests' did, this rule stops
 *     seeing them and says nothing about it. Clause 2 is what catches that: the
 *     standing wording disappearing from the route fails, loudly, and the fix is
 *     to widen the corpus rather than to delete the assertion.
 * ========================================================================== */

/** §17.4's own text: rule 7's section, sliced out of the §17 already read above. */
const s174Body = (() => {
	const at = s17Body.indexOf('\n### 17.4 ');
	if (at === -1) return '';
	const rest = s17Body.slice(at);
	const end = rest.indexOf('\n### ', 10);
	return end === -1 ? rest : rest.slice(0, end);
})();

/**
 * Rule 7 itself. Its numbered item runs unbroken to the blank line that ends it,
 * which is what bounds it here — taking the rest of §17.4 would sweep in the
 * "Every result row is one template" paragraph and its backticked column lists.
 */
const RULE7_HEAD = "7. **Search's own copy never enumerates media types";
const rule7 = (() => {
	const at = s174Body.indexOf(RULE7_HEAD);
	if (at === -1) return '';
	const end = s174Body.indexOf('\n\n', at);
	return end === -1 ? s174Body.slice(at) : s174Body.slice(at, end);
})();

/**
 * §17's standing wording for this screen, read live out of rule 7's `*"…"*` span.
 *
 * ⚠️ THE ITALIC-QUOTED FORM IS LOAD-BEARING AND IS §17'S OWN CONVENTION, not a
 * parsing convenience. §17.4 says so where it retires the old string: "That is in
 * backticks and not in this section's italic-quoted form, because the
 * italic-quoted form marks copy that ships and this string is retired." So the
 * span this reads is exactly the span §17 marks as shipping copy, and the
 * retired one is structurally excluded from being read as the standard.
 */
const STANDING_WORDING = (() => {
	const m = /\*"([\s\S]*?)"\*/.exec(rule7);
	return m ? collapse(m[1]) : '';
})();

/**
 * The string this rule exists because of, read out of rule 7's BACKTICKS — the
 * form §17 uses for copy that is retired.
 *
 * It is the drill's probe below, and reading it rather than retyping it is the
 * point: a probe typed from memory drifts from the string it claims to be, and
 * then the drill proves the rule fires on something nobody ever shipped.
 */
const RETIRED_ENUMERATION = (() => {
	const spans = [...rule7.matchAll(/`([^`]+)`/g)].map((m) => collapse(m[1]));
	return spans.find((s) => /\bfilm\b/i.test(s) && /\bcomic\b/i.test(s)) ?? '';
})();

/**
 * The words that name a media type, keyed by the type they name.
 *
 * The KEYS are `MEDIA_TYPES`, imported rather than restated, so the six cannot
 * drift apart and a seventh fails the coverage test. The VALUES are English and
 * are hand-written; see the limits above.
 */
const MEDIA_TYPE_WORDS: Record<MediaType, readonly string[]> = {
	movies: ['film', 'films', 'movie', 'movies'],
	tv: ['episode', 'episodes', 'tv'],
	music: ['album', 'albums', 'music', 'song', 'songs'],
	ebooks: ['book', 'books', 'ebook', 'ebooks', 'e-book', 'e-books'],
	audiobooks: ['audiobook', 'audiobooks', 'audio-book', 'audio-books'],
	comics: ['comic', 'comics']
};

const WORD_TO_TYPE: ReadonlyMap<string, MediaType> = new Map(
	Object.entries(MEDIA_TYPE_WORDS).flatMap(([type, words]) =>
		words.map((w) => [w, type as MediaType] as const)
	)
);

/**
 * One alternation, LONGEST ALTERNATIVE FIRST, and that ordering is the whole
 * reason this is one regex rather than a loop over the words.
 *
 * `audiobooks` contains `books` and `ebooks` contains `books`. Matched
 * separately, "audiobooks" would count as BOTH audiobooks and ebooks, and a
 * string naming one type would read as an enumeration of two — a false positive
 * on the strictest possible input, which is how a copy rule earns its first
 * exception. Sorted by length, the engine consumes `audiobooks` at that position
 * and `books` never sees it. `\b` handles the other direction: `book` does not
 * match inside `bookmark`.
 */
const MEDIA_TYPE_WORD = new RegExp(
	`\\b(${[...WORD_TO_TYPE.keys()].sort((a, b) => b.length - a.length).join('|')})\\b`,
	'gi'
);

/** The media types one string names, in `MEDIA_TYPES` order so a report is stable. */
function typesNamed(text: string): MediaType[] {
	const found = new Set<MediaType>();
	const re = new RegExp(MEDIA_TYPE_WORD.source, 'gi');
	let m: RegExpExecArray | null;
	while ((m = re.exec(text)) !== null) {
		const t = WORD_TO_TYPE.get(m[1].toLowerCase());
		if (t) found.add(t);
	}
	return MEDIA_TYPES.filter((t) => found.has(t));
}

/** §17's own threshold: a list of two is a list. */
const ENUMERATION_MIN = 2;

/**
 * The failure line. File, line, source position, WHICH types were named and the
 * offending text, because "no enumeration" tells the person reading CI nothing
 * they can act on — they need to know which words tripped it to know what to cut.
 */
const reportEnumeration = (s: CopyString, types: readonly MediaType[]): string =>
	`${s.file}:${s.line}  ${s.source}  names ${types.length} media types ` +
	`(${types.join(', ')})  "${s.text.slice(0, 160)}"`;

/** Search's own copy, which is rule 7's corpus and nothing wider. */
const SEARCH_ROUTE = 'web/src/routes/search/';
const SEARCH_COPY: readonly CopyString[] = COPY.filter((s) => s.file.startsWith(SEARCH_ROUTE));

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
 * ⚠️ THE FLOOR WAS RE-DERIVED ON 2026-08-17, OFF A DIFFERENT QUANTITY. The first
 * version of this rule set the floor at 0.2064 = the palette's GLOBAL MAXIMUM
 * CHROMA (0.179274, `--status-error` light) plus a margin. Design read the code
 * and rejected the derivation, and the reason is the conjunction above:
 *
 *     `--status-error` is at hue 29.0°. It is not in [265°, 335°], so it never
 *     reaches the chroma test at all. A value the chroma test never sees cannot
 *     constrain the chroma test's floor.
 *
 * The floor was doing a job nothing asked of it. Under a conjunction the ONLY
 * thing the floor has to do is refuse the one false positive the hue test can
 * produce on its own: a NEAR-ACHROMATIC value has no meaningful hue, because at
 * chroma near zero the a/b coordinates are numerical dust and `atan2` of dust
 * returns an angle that can land anywhere, including inside the band. That is
 * the whole of it. So the floor is derived from HUE STABILITY instead: it must
 * sit above the chroma at which a computed hue stops being noise.
 *
 * THE FLOOR IS MEASURED, NOT CHOSEN. Design required this explicitly, and the
 * measurement is reproduced live by the first two rule-2 tests below so it
 * cannot go stale the way a quoted number does.
 *
 * WHAT WAS MEASURED — THE NEUTRAL RAMPS, IN BOTH FILES. `docs/design/tokens.css`
 * and `web/src/app.css`, every `--n-0`..`--n-8` step plus the `--inset` and
 * `--hover` interstitials, in every block: §1 `:root` (light), §2
 * `:root[data-theme="dark"]` and §3's `prefers-color-scheme: dark`. 33 values
 * per file, 66 in all, 0 skipped. These are the values whose hue is meaningless
 * by construction, so they are the population the floor has to clear. Comments
 * are stripped first, so the withdrawn and rejected candidates tokens.css
 * records in prose (`#8a5300`, `#a9700a`, `#e0a33a`, the withdrawn
 * `--protocol-*` pair, CSS `orange` quoted as a hue reference) are correctly NOT
 * counted as tokens.
 *
 *       MEASURED BASIS   0.024372
 *       (--n-4 light, #807869, OKLCH L 0.575487 / C 0.024371 / H 83.228°;
 *        the identical value is the maximum in BOTH files)
 *
 * The constant below is 0.024372 rather than 0.024371 because the exact value is
 * 0.024371490... and a bound the floor must CLEAR is rounded up, not to nearest.
 *
 * Runners-up, so the basis is visibly a maximum and not a lone spike: `--n-5`
 * light `#6b6355` at 0.023764, `--n-5` dark `#9a9284` at 0.022314, `--n-4` dark
 * `#7d7568` at 0.021834. The ramp's floor is `--inset` light `#fdfcfb` at
 * 0.001703, whose hue of 67.8° is exactly the dust this floor is about: three
 * hex digits' difference from `#ffffff`, and an "orange" hue nobody chose.
 *
 * THE MARGIN, ALSO MEASURED, AND UNCHANGED BY THE RE-DERIVATION. A floor sitting
 * flush on today's basis fails the first time a legitimate token is retuned, so
 * the margin is the largest chroma move this palette has ACTUALLY made, taken
 * from tokens.css's own recorded history rather than from a comfortable round
 * number:
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
 * nudging the floor. ⚠️ AND IT IS A STATUS-COLOUR MARGIN APPLIED TO A NEUTRAL:
 * tokens.css records no retune of a ramp step at all, so this is the largest
 * move any token has made, standing in for the largest move a ramp step could
 * make. That is a deliberate over-estimate, which is the safe direction here.
 *
 * THE FLOOR:
 *
 *       0.024372 (measured basis) + 0.0271 (margin) = 0.051472
 *       CHROMA FLOOR = 0.0515, rounded UP, which is the safe direction.
 *
 * ALL THREE NUMBERS ARE STATED SEPARATELY BECAUSE THE DERIVATION HAS TO BE
 * VISIBLE, not implied: a reader can see that the floor clears the most
 * chromatic neutral either file ships by 0.0271, and that it sits 0.0498 below
 * `#c4b5fd` violet-300, the palest Tailwind violet anyone actually reaches for.
 * Both halves matter. A floor that fails the first is a false-positive machine;
 * a floor that fails the second is the 0.2064 floor again.
 *
 * ─────────────────────────────────────────────────────────────────────────────
 * ⚠️ THE FLOOR RESTS ON THE BAND BEING EMPTY, SO THAT IS ASSERTED, WITH ROOM.
 * The 0.0515 floor is BELOW four real tokens: `--status-error` in both themes,
 * `--status-warn` in both, `--status-ok` in both. They pass rule 2 purely
 * because their hue is nowhere near the band. Under the old 0.2064 floor a token
 * drifting into the band would still have cleared on chroma; under this one it
 * would fail the moment it crossed 265°, which is a real (if remote) way for
 * this rule to start failing a legitimate colour.
 *
 * So the proximity guard below asserts that no token in tokens.css comes within
 * 40° of the band, and says "re-derive the floor" rather than "banned colour" if
 * one ever does. It is measured live, like everything else here.
 *
 * ⚠️ AND THE ROOM IS 50.84°, NOT THE 173° THIS COMMENT CLAIMED BEFORE. That
 * figure was wrong, and it was wrong in the direction that flatters the rule, so
 * it is corrected here rather than quietly replaced. HUE IS CIRCULAR. The old
 * text took `--n-1` dark at hue 91.6° and subtracted it from the band's lower
 * edge (265 - 91.6 = 173.4), which measures the gap in one direction only. The
 * band's upper edge is at 335°, and the short way round from 335° runs through
 * 0° and reaches 25.8° in 50.8°. The genuinely nearest token is therefore
 * `--status-error` dark `#f0837a` at hue 25.841°, 50.84° from the band, and the
 * 40° guard clears by 10.84° rather than by 133°. It still passes. It is
 * tighter than advertised, and a warm red palette is exactly the kind that
 * approaches a magenta band from below zero, so the guard earns its place.
 *
 * ─────────────────────────────────────────────────────────────────────────────
 * ⚠️ WHAT THIS FLOOR CATCHES, AND WHAT IT LETS THROUGH. Honesty about the cost
 * is worth more than a rule that reads as though it catches everything, and the
 * costs moved when the floor did.
 *
 * CAUGHT (chroma >= 0.0515, hue in band). The whole saturated range the 0.2064
 * floor caught, and now the two families it did not: the mid tints, and the
 * named CSS purples that were relying on rule 1 to catch them by word.
 *
 *       #7c3aed  violet-600    C 0.2466  H 293.0°     #9333ea  purple-600   C 0.2525  H 302.3°
 *       #4f46e5  indigo-600    C 0.2301  H 277.0°     #c026d3  fuchsia-600  C 0.2569  H 322.9°
 *       #9400d3  darkviolet    C 0.2607  H 309.8°     #ff00ff  CSS fuchsia  C 0.3225  H 328.4°
 *
 *   NEWLY CAUGHT, each of which the 0.2064 floor let through:
 *
 *       #6366f1  indigo-500    C 0.2041  H 277.1°   missed the old floor by 0.0023
 *       #800080  CSS purple    C 0.1935  H 328.4°   } were rule-1-only, by name
 *       #4b0082  CSS indigo    C 0.1793  H 301.7°   } now banned by value too, so
 *       #ee82ee  CSS violet    C 0.1861  H 327.2°   } the hex form no longer walks
 *       #da70d6  CSS orchid    C 0.1813  H 328.7°   was caught by NEITHER rule;
 *                                                  now caught by both
 *       #c4b5fd  violet-300    C 0.1013  H 293.6°   } the tints, which the old
 *       #a5b4fc  indigo-300    C 0.1041  H 274.7°   } comment recorded as being
 *       #ddd6fe  violet-200    C 0.0549  H 293.3°   } out of reach of ANY floor
 *       #c7d2fe  indigo-200    C 0.0622  H 274.0°   }
 *
 * STILL NOT CAUGHT BY RULE 2, and the line is now where the noise floor is
 * rather than where the palette's maximum happened to be:
 *
 *       #ede9fe  violet-100    C 0.0284  H 294.6°   the palest washes only
 *       #f5f3ff  violet-50     C 0.0161  H 293.8°
 *       #d8bfd8  CSS thistle   C 0.0439  H 326.0°   } rule 1 catches these two
 *       #e6e6fa  CSS lavender  C 0.0269  H 285.9°   } by NAME, and only by name
 *
 * Three things follow, and they are the reason this list is here rather than
 * summarised away:
 *
 *   1. TINTS ARE NO LONGER OUT OF REACH, AND THE OLD NOTE SAYING THEY WERE HAS
 *      BEEN DELETED RATHER THAN SOFTENED. It said "no floor derived from the
 *      palette can reach them without banning the palette", which was true only
 *      of a floor derived from the palette's MAXIMUM. A floor derived from the
 *      palette's noise level reaches violet-200 with room to spare and bans
 *      nothing, because the palette's neutrals are 0.0271 below it and the
 *      palette's chromatic tokens are 236° of hue away from the test.
 *      What survives is a much smaller version of the same limit: below about
 *      0.05 a violet wash and a cool grey are genuinely hard to tell apart by
 *      chroma, so violet-100 and violet-50 stay out of reach. That residue is
 *      accepted, and it is where the false-positive risk actually lives.
 *   2. `#4b0082` IS AN ARRESTING COINCIDENCE AND NOTHING MORE. CSS `indigo`
 *      measures chroma 0.17927151; `--status-error` measures 0.17927323. They
 *      differ by 1.7e-6, at hues 272.7° apart — the most saturated colour this
 *      design system uses and the colour §13 is named after are, to five decimal
 *      places, the same distance from grey. It USED to be load-bearing: under
 *      the 0.2064 floor it was the proof that the floor could not be lowered to
 *      catch CSS `indigo` without failing a real token. That argument was the
 *      derivation error in miniature, and the re-derivation dissolves it, since
 *      the two values were never compared by the rule in the first place. It is
 *      kept because a reader WILL notice it, and it is still the sharpest
 *      illustration going of why chroma alone is not a purple test.
 *   3. RULE 1 AND RULE 2 COMPOSE, AND THE KEYWORD-SHAPED HOLE IS NOW CLOSED.
 *      Rule 2 does not resolve CSS colour keywords at all, and never will — a
 *      keyword table is rule 1's shape of problem, not rule 2's. The gap used to
 *      be that `orchid`, `plum` and `magenta` were on NEITHER list; their values
 *      closed first (`#da70d6` and `#dda0dd` both clear 0.0515 inside the band),
 *      and rule 1's widening to check.mjs's nine closed the words.
 *      `thistle` #d8bfd8 and `lavender` #e6e6fa are the pair that shows why the
 *      word half has to exist at all rather than being an overlap with rule 2.
 *      At chroma 0.0439 and 0.0269 they are UNDER the floor, and they are under
 *      it for the reason item 1 gives: below about 0.05 a pale wash and a cool
 *      grey are not separable by chroma, so any floor low enough to catch these
 *      two would read the neutral ramps as purple. They are violet-100's case
 *      with a CSS name on it — structurally out of reach of any chroma test,
 *      caught by the word list or not at all.
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

/**
 * The floor and the two numbers it is made of, all three re-measured live below.
 * See the block comment: the basis is the most chromatic NEUTRAL either file
 * ships, because under the conjunction the floor's only job is to refuse a hue
 * computed from numerical dust.
 */
const NEUTRAL_MAX_CHROMA = 0.024372;
const CHROMA_MARGIN = 0.0271;
const CHROMA_FLOOR = 0.0515;

/**
 * `#c4b5fd` violet-300, the palest violet a generated UI actually reaches for,
 * and the ceiling the floor has to stay well under to be worth having. Asserted
 * against the floor below rather than left as a claim in prose.
 */
const TINT_REFERENCE_CHROMA = 0.1013;

/**
 * How close a real token may come to the band before the floor stops being the
 * right floor. The derivation assumes the band is EMPTY; 40° is the width of the
 * warning strip either side of it. Today's nearest token clears by 10.84°.
 */
const BAND_PROXIMITY_MIN = 40;

const inBand = (c: Oklch): boolean => c.H >= BAND_LO && c.H <= BAND_HI;
const banned = (v: ColourValue): boolean => inBand(v.oklch) && v.oklch.C >= CHROMA_FLOOR;

/**
 * Shortest angular separation between two hues.
 *
 * ⚠️ THE WRAP IS THE POINT. `Math.abs(a - b)` reads 309° for a pair 51° apart
 * across 0°, which is how this file's own comment came to claim 173° of room
 * where there is 50.84°. Hue is an angle; measure it like one.
 */
const arc = (a: number, b: number): number => {
	const d = (((a - b) % 360) + 360) % 360;
	return Math.min(d, 360 - d);
};

/** Degrees from the banned band, either side, and 0 for anything inside it. */
const bandDistance = (c: Oklch): number =>
	inBand(c) ? 0 : Math.min(arc(c.H, BAND_LO), arc(c.H, BAND_HI));

/**
 * The proximity guard design asked for, as a function so the measurement test
 * can run it over the real tokens and the drill can fire it on a planted one.
 * A guard that has only ever been run on input that passes is not a guard.
 */
const nearBand = (values: readonly ColourValue[]): ColourValue[] =>
	values.filter((v) => bandDistance(v.oklch) < BAND_PROXIMITY_MIN);

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

/** The proximity guard's own line, which reports a distance rather than a verdict. */
const reportProximity = (v: ColourValue): string =>
	`${v.file}:${v.line}  ${v.where || '(no enclosing selector)'}  ` +
	`${v.decl} ${v.text}  ` +
	`OKLCH hue ${v.oklch.H.toFixed(2)}°, chroma ${v.oklch.C.toFixed(4)}  ` +
	`${bandDistance(v.oklch).toFixed(2)}° from the band ${BAND_LO}°-${BAND_HI}°`;

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
		expect(corpusShortfall(FILES)).toBeNull();
	});

	it('the corpus floors fire on a starved corpus, and on a lost directory', () => {
		/* THE FLOORS DRILLED, not assumed. Both of them had gone quiet at some point
		   — the character floor by the tree growing past it — and a floor nobody has
		   watched fail is a number, not a guard. Fired on the real predicate the
		   read and the test above both consult. */
		expect(corpusShortfall(FILES.slice(0, FILE_FLOOR - 1))).toMatch(/below the floor of 30/);

		/* THE CHARACTER FLOOR ON THE PROPERTY ITS DERIVATION CLAIMS — losing the
		   single largest file must fail — fired against the real corpus with that
		   one file removed, so the number above is checked rather than believed.
		   This is the assertion that was silently false before the re-derivation. */
		const largest = [...FILES].sort((a, b) => b.src.length - a.src.length)[0];
		const lost = FILES.filter((f) => f.file !== largest.file);
		expect(
			lost.length,
			'removing the largest file must not trip the FILE floor'
		).toBeGreaterThanOrEqual(FILE_FLOOR);
		expect(corpusShortfall(lost), `losing ${largest.file} did not trip the floor`).toMatch(
			/below the floor of 1100000/
		);

		/* And the negative half — the real corpus must NOT trip either floor, or the
		   two assertions above pass for the wrong reason. */
		expect(corpusShortfall(FILES)).toBeNull();
	});

	it('fails on a file that moved under the walk, once, and does not read on without it', () => {
		/* THE SECOND FAILURE MODE, DRILLED AS FAR AS IT HONESTLY CAN BE. A concurrent
		   merge cannot be summoned on demand, so what is simulated is not the tree
		   but the FILESYSTEM's answer — one ENOENT between the readdir and the read,
		   which is exactly what the walk sees either way. What is NOT claimed is
		   coverage of a real concurrent rewrite; that is unreproducible here.

		   Three properties, and the third is the ruling. The read must turn red; it
		   must say the failure is not the gated diff, so it is not misattributed to
		   whoever was unlucky enough to be gating; and it must NOT try again —
		   `attempts` is 1, pinned, because a loop that eventually gets a clean pass
		   has read several incoherent trees rather than one coherent one. */
		const flakyOnce = (): { io: CorpusIO; walks: () => number } => {
			let walks = 0;
			return {
				walks: () => walks,
				io: {
					list: (dir) => {
						if (dir === SRC) walks++;
						return nodeIO.list(dir);
					},
					isDir: nodeIO.isDir,
					read: (p) => {
						if (walks === 1 && p.endsWith('app.css')) {
							throw new Error(`ENOENT: no such file or directory, open '${p}'`);
						}
						return nodeIO.read(p);
					}
				}
			};
		};

		/* The ENOENT is thrown on the FIRST walk only, so a read that tried again
		   would succeed and return a corpus — which is precisely the green this
		   must not produce. It turning red is the no-retry property. */
		const first = flakyOnce();
		expect(() => readCorpus(first.io)).toThrow(/could not be read whole/);
		expect(first.walks(), 'the read walked twice — a retry is the fix design ruled out').toBe(1);

		const second = flakyOnce();
		expect(() => readCorpus(second.io)).toThrow(/it is not the diff being gated/);
	});

	it('fails on a corpus that came back too small, which is the case that would be green', () => {
		/* THE DEAF-GUARD CASE, and the reason the floors are consulted by the READ
		   and not only by the test above. A tree that lists almost nothing raises no
		   error at all: every read succeeds, the corpus is simply tiny, and every
		   rule reports a clean sheet. Nothing is red anywhere. */
		const emptied: CorpusIO = {
			list: (dir) => (dir === SRC ? ['app.css'] : []),
			isDir: () => false,
			read: nodeIO.read
		};
		expect(() => readCorpus(emptied)).toThrow(/below the floor of 30/);
		expect(() => readCorpus(emptied)).toThrow(/would otherwise have gone GREEN/);
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

	/* The gate's nine, transcribed whole from `docs/design/check.mjs` — the four
	   families §13 names, plus the five CSS keywords that ARE those families
	   under another name: `orchid` #da70d6, `plum` #dda0dd, `magenta` #f0f (the
	   `fuchsia` synonym), and the family's two pale tints, `thistle` #d8bfd8 and
	   `lavender` #e6e6fa.

	   THE TINTS ARE WHY THIS RULE CANNOT BE FOLDED INTO RULE 2, and they are the
	   same residual rule 2's table already records, reached from the other side:
	   at chroma 0.0439 and 0.0269 they sit below the 0.0515 floor, and below any
	   floor that does not also ban the neutral ramps — which is exactly what
	   keeps violet-100 out of rule 2's reach there. No chroma threshold can
	   catch a colour this pale without catching legitimate greys, so for these
	   two the word list is not a convenience, it is the only guard there is.

	   `\b` is what makes the widening free: it already declines `plumbing`,
	   `plummet`, `magentaBright`, `lavenderblush`, `thistledown`, `whistle` and
	   `bristle`, and none of the nine occurs anywhere in the corpus today. */
	it('§13 colour: no indigo / violet / purple / fuchsia / orchid / plum / magenta / lavender / thistle', () => {
		const { hits } = applyRule(
			/\b(indigo|violet|purple|fuchsia|orchid|plum|magenta|lavender|thistle)\b/i
		);
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

	it("§13 colour by value: the floor's basis, re-measured off both neutral ramps", () => {
		/* THE FLOOR'S PREMISE, RE-MEASURED RATHER THAN QUOTED. A number written into
		   a comment goes stale silently — tokens.css §0 carries a whole note about a
		   restated fact that stayed wrong for months (REVIEW-LOG SD-01), and the 173°
		   this file itself claimed until the re-derivation is a second instance. So
		   the measurement runs on every `make check` instead. */
		const files = [
			{ path: TOKENS_CSS, name: 'docs/design/tokens.css' },
			{ path: join(SRC, 'app.css'), name: 'web/src/app.css' }
		];
		const values: ColourValue[] = [];
		const skips: ColourSkip[] = [];
		for (const f of files)
			readColours(strip(readFileSync(f.path, 'utf8'), 'css'), f.name, values, skips);

		/* 92 values today, 46 per file over three theme blocks of 15 (nine ramp steps,
		   two interstitials, four status) plus one overlay each. Floor 64: losing any
		   one theme block drops the count to 77 and fails, which is the regression a
		   count floor is for. Not rounded to 60, which two lost blocks would survive. */
		expect(
			values.length,
			`only ${values.length} colour value(s) read out of tokens.css and app.css, below ` +
				`the floor of 64. The basis below is measured from those files, so a parse that ` +
				`finds nothing would "measure" a maximum chroma of zero and pass this trivially.`
		).toBeGreaterThanOrEqual(64);
		expect(
			[...new Set(skips.map((s) => `${s.file}  ${s.text}`))].sort(),
			'a colour in tokens.css or app.css could not be converted, so the measured basis ' +
				'is not a maximum over the whole of either file. Convert it, or record it and ' +
				're-derive the basis by hand.\n' +
				skips.map((s) => `${s.file}:${s.line}  ${s.text}  ${s.why}`).join('\n')
		).toEqual(Object.keys(COLOUR_SKIPS).sort());

		/* PREMISE 1: the band is empty, in both files. Stated as an assertion, not as
		   prose, so the day a purple token lands the whole derivation is reconsidered. */
		const band = values.filter((v) => inBand(v.oklch));
		expect(
			band.map(reportColour),
			`a token now falls in the banned hue band ${BAND_LO}°-${BAND_HI}°. The floor is ` +
				`BELOW the four status colours, so an in-band token would now FAIL rule 2. ` +
				`Re-derive before doing anything else.`
		).toEqual([]);

		/* PREMISE 2: nothing is even NEAR the band. This is the guard design asked
		   for, and it is the one that gives premise 1 somewhere to fail early. The
		   distance is circular: today's nearest is --status-error dark at hue 25.8°,
		   which is 50.84° from the band's upper edge THE SHORT WAY ROUND, not the
		   173° a subtraction from the lower edge reports. */
		expect(
			nearBand(values).map(reportProximity),
			`a token is now within ${BAND_PROXIMITY_MIN}° of the banned band ` +
				`${BAND_LO}°-${BAND_HI}°. Nothing is banned yet, and nothing here is wrong ` +
				`with the token: this fires early, on purpose. The chroma floor of ` +
				`${CHROMA_FLOOR} was derived on the basis that the palette lives nowhere near ` +
				`the band, and a token approaching it means the floor needs RE-DERIVING off ` +
				`the in-band population rather than off the neutral ramp. Do not widen ` +
				`${BAND_PROXIMITY_MIN}° to make this green.`
		).toEqual([]);

		/* THE BASIS: the most chromatic neutral either file ships. `--n-N` plus the
		   two interstitials, which is the population whose hue is meaningless by
		   construction and therefore the population the floor exists to clear. */
		const ramp = values.filter((v) => /^--(n-\d|inset|hover)\b/.test(v.decl));
		expect(
			ramp.length,
			`only ${ramp.length} neutral-ramp value(s) matched across both files, below the ` +
				`floor of 44. Eleven per theme block, three blocks, two files. If the ramp ` +
				`was renamed, this measures a smaller set and reports a lower basis, which ` +
				`would loosen the floor silently.`
		).toBeGreaterThanOrEqual(44);

		const measured = Math.max(...ramp.map((v) => v.oklch.C));
		const worst = ramp.find((v) => v.oklch.C === measured)!;
		expect(
			measured,
			`the neutral ramps' maximum chroma is now ${measured.toFixed(6)} ` +
				`(${reportColour(worst)}), above the ${NEUTRAL_MAX_CHROMA} the floor of ` +
				`${CHROMA_FLOOR} was derived from. A more chromatic neutral means a higher ` +
				`noise level, so re-derive: floor = measured basis + margin ${CHROMA_MARGIN}.`
		).toBeLessThanOrEqual(NEUTRAL_MAX_CHROMA);
		expect(
			measured,
			'the neutral ramps lost their most chromatic step, so the floor is now looser ' +
				'than the measurement it claims to rest on'
		).toBeGreaterThan(NEUTRAL_MAX_CHROMA - 0.001);

		/* And the arithmetic that turns the two into the floor, asserted rather than
		   trusted to the comment that states it. */
		expect(
			CHROMA_FLOOR,
			`the floor ${CHROMA_FLOOR} no longer clears the measured basis ` +
				`${NEUTRAL_MAX_CHROMA} by the stated margin ${CHROMA_MARGIN}`
		).toBeGreaterThanOrEqual(NEUTRAL_MAX_CHROMA + CHROMA_MARGIN);

		/* THE OTHER HALF OF THE DERIVATION, WHICH IS THE HALF THE 0.2064 FLOOR FAILED.
		   A floor above a real tint catches nothing anybody ships. `#c4b5fd` is the
		   ceiling, re-measured here rather than quoted so the constant cannot drift
		   from the colour it names. */
		const tint = parseHex('#c4b5fd');
		expect(tint.ok, 'the tint reference failed to parse').toBe(true);
		if (!tint.ok) return;
		expect(tint.oklch.C, 'violet-300 is not the chroma TINT_REFERENCE_CHROMA records').toBeCloseTo(
			TINT_REFERENCE_CHROMA,
			4
		);
		expect(
			CHROMA_FLOOR,
			`the floor ${CHROMA_FLOOR} has risen to within 0.02 of violet-300 ` +
				`(${TINT_REFERENCE_CHROMA}), so the tints this rule was re-derived to catch are ` +
				`slipping back out of reach. Re-derive rather than accepting the gap.`
		).toBeLessThan(TINT_REFERENCE_CHROMA - 0.02);
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

	/** One planted declaration through the real extractor, not a re-implementation. */
	const plant = (value: string): ColourValue => {
		const values: ColourValue[] = [];
		const skips: ColourSkip[] = [];
		readColours(`.probe {\n\t--accent: ${value};\n}\n`, 'web/src/planted.css', values, skips);
		expect(skips, `the planted value ${value} was skipped instead of converted`).toEqual([]);
		expect(values.length, `the planted value ${value} was not read as a colour`).toBe(1);
		return values[0];
	};

	it('§13 colour by value: the re-derived floor, drilled by mechanism and not by verdict', () => {
		/* ⚠️ WHY THIS TABLE REPORTS TWO BOOLEANS AND NOT ONE VERDICT. A probe that
		   passes because a DIFFERENT condition absorbed it looks identical to a probe
		   that passes because the rule works — that is exactly how the 0.2064 floor
		   survived review while resting on a token the chroma test never sees. So
		   every row states WHICH half fired: `band` is the hue test, `floor` is the
		   chroma test, and `banned` is their conjunction. Read the rows, not the
		   verdict. */
		const probes: readonly { readonly what: string; readonly value: string }[] = [
			/* Unchanged by the re-derivation: the value the rule was written for. */
			{ what: 'violet-600, the original gap', value: '#7c3aed' },
			/* The three the 0.2064 floor let through, which is the point of the change. */
			{ what: 'indigo-500, missed the old floor by 0.0023', value: '#6366f1' },
			{ what: 'violet-300, a tint the old comment called unreachable', value: '#c4b5fd' },
			{ what: 'violet-200, the palest tint now in reach', value: '#ddd6fe' },
			/* Below the noise floor, and honestly out of reach. */
			{ what: 'violet-100, still under the floor', value: '#ede9fe' },
			/* The false positives the floor exists to prevent. */
			{ what: 'a cool neutral at hue 314.7°', value: '#4a464c' },
			{ what: "Tailwind zinc-500, somebody else's cool ramp", value: '#71717a' },
			/* In-band-adjacent but out of band: the hue test alone must reject them. */
			{ what: 'the palette itself, --status-error light', value: '#b3251c' }
		];

		const rows = probes.map((p) => {
			const v = plant(p.value);
			return {
				what: p.what,
				band: inBand(v.oklch),
				floor: v.oklch.C >= CHROMA_FLOOR,
				banned: banned(v)
			};
		});

		expect(rows, 'a probe changed which half of the conjunction decided it').toEqual([
			/* hue 293.0°, chroma 0.2466. Both halves fire; banned, as before. */
			{ what: 'violet-600, the original gap', band: true, floor: true, banned: true },
			/* hue 277.1°, chroma 0.2041. Band fired before AND now; the FLOOR is what
			   changed, from 0.2064 (fails) to 0.0515 (clears). */
			{ what: 'indigo-500, missed the old floor by 0.0023', band: true, floor: true, banned: true },
			/* hue 293.6°, chroma 0.1013. Same story, and the larger one: 0.1013 clears
			   0.0515 by 0.0498 and missed 0.2064 by 0.1051. */
			{
				what: 'violet-300, a tint the old comment called unreachable',
				band: true,
				floor: true,
				banned: true
			},
			/* hue 293.3°, chroma 0.0549. Clears by 0.0034, the narrowest pass here, and
			   said so rather than presented as comfortable. */
			{ what: 'violet-200, the palest tint now in reach', band: true, floor: true, banned: true },
			/* hue 294.6°, chroma 0.0284. In band, under the floor: NOT banned, and the
			   row is here so the residual gap is drilled rather than only described. */
			{ what: 'violet-100, still under the floor', band: true, floor: false, banned: false },
			/* hue 314.7°, chroma 0.0112. In band. The FLOOR is the only thing between
			   this legitimate grey and a false positive. */
			{ what: 'a cool neutral at hue 314.7°', band: true, floor: false, banned: false },
			/* hue 285.9°, chroma 0.0138. Same mechanism, somebody else's palette. */
			{
				what: "Tailwind zinc-500, somebody else's cool ramp",
				band: true,
				floor: false,
				banned: false
			},
			/* hue 29.0°, chroma 0.1793. ⚠️ `floor: true`. This token is ABOVE the floor
			   and passes ONLY because the hue test rejected it first. It is the whole
			   re-derivation in one row: the value the old floor was built to clear never
			   reaches the chroma test, so it never constrained it. */
			{ what: 'the palette itself, --status-error light', band: false, floor: true, banned: false }
		]);
	});

	it('§13 colour by value: a near-grey rotated INTO the band still does not fire', () => {
		/* THE FALSE POSITIVE THE FLOOR EXISTS TO PREVENT, CONSTRUCTED RATHER THAN
		   FOUND. `#4a464c` above is a near-grey that happens to land in the band, but
		   it is somebody's arbitrary hex and clears the floor by 0.0403, which makes
		   it an easy probe. So here is the hardest one this palette can produce:
		   take --n-4 light, the MOST chromatic neutral either file ships and the exact
		   value the basis was measured from, and rotate its hue to 300°, the middle of
		   the band, keeping L and C. It is now a colour that is simultaneously the
		   noisiest hue this design system can generate and sitting dead centre in the
		   banned band. If the floor is derived correctly it must not fire, and it must
		   miss by exactly the stated margin. */
		const worstNeutral = plant(`oklch(0.575487 ${NEUTRAL_MAX_CHROMA} 300)`);

		expect(inBand(worstNeutral.oklch), 'the constructed probe is not in the band').toBe(true);
		expect(worstNeutral.oklch.H, 'the probe should sit mid-band').toBeCloseTo(300, 6);
		expect(
			banned(worstNeutral),
			'the most chromatic neutral this palette ships, rotated into the band, reads as ' +
				'purple. The floor is too low: it is now inside the noise it was derived to ' +
				'sit above, and every cool grey is a false positive waiting to happen.'
		).toBe(false);

		/* And the margin, asserted as a quantity rather than left implied by the
		   boolean above. This is the number the whole derivation is: how far the
		   worst legitimate near-grey sits below the floor. */
		expect(
			CHROMA_FLOOR - worstNeutral.oklch.C,
			`the constructed near-grey clears the floor by ${(CHROMA_FLOOR - worstNeutral.oklch.C).toFixed(6)}, ` +
				`not the stated margin of ${CHROMA_MARGIN}`
		).toBeGreaterThanOrEqual(CHROMA_MARGIN);
	});

	it('§13 colour by value: the band-proximity guard fires on a token near the edge', () => {
		/* THE GUARD, DRILLED. It passes over the real tokens by 10.84°, and a guard
		   that has only ever seen input that passes is indistinguishable from no
		   guard — the same argument that put the planted `#7c3aed` above in the file.
		   240° is 25° below the band's lower edge: NOT banned, because nothing about
		   it is in the band, but close enough that the floor's premise is wobbling. */
		const approaching = plant('oklch(0.55 0.15 240)');
		expect(inBand(approaching.oklch), 'the probe must be OUTSIDE the band').toBe(false);
		expect(banned(approaching), 'the probe must not be banned, only near').toBe(false);
		expect(bandDistance(approaching.oklch), 'the probe should sit 25° out').toBeCloseTo(25, 4);
		expect(nearBand([approaching]).length, 'the proximity guard did not fire').toBe(1);
		expect(reportProximity(approaching)).toContain('25.00° from the band');

		/* Both sides of the band, because a one-sided distance is the bug this guard
		   was written after. 5° above the upper edge, reached the short way round
		   through 0°, is where --status-error's family lives. */
		const above = plant('oklch(0.55 0.15 340)');
		expect(
			bandDistance(above.oklch),
			'the wrap-around side of the band is not measured'
		).toBeCloseTo(5, 4);
		expect(nearBand([above]).length, 'the proximity guard is one-sided').toBe(1);

		/* And the negative half: a token comfortably clear must NOT fire, or the guard
		   is just an assertion that always fails. --status-ok dark is 111.65° out. */
		const clear = plant('#5cba7d');
		expect(bandDistance(clear.oklch)).toBeGreaterThan(BAND_PROXIMITY_MIN);
		expect(nearBand([clear]), 'the guard fires on a token nowhere near the band').toEqual([]);
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
				`so this is the half that catches a literal. The floor is MEASURED off the ` +
				`neutral ramps (basis ${NEUTRAL_MAX_CHROMA} + margin ${CHROMA_MARGIN} = ` +
				`${CHROMA_FLOOR}), so anything failing here is more chromatic than the most ` +
				`chromatic grey this design system ships, and sits in the purple band. It is ` +
				`a colour, not noise. Do not raise the floor to make this green.`
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

	/* check.mjs's CENTER pattern. The `where` group is the mechanism, not
	   decoration: the exemption is about WHERE the declaration sits, so the
	   pattern captures the selector (CSS) or the element and its attributes (an
	   inline style) and hands THAT to the exemption.

	   ASSEMBLED FROM `CENTRED` RATHER THAN TYPED AROUND IT, so the anchor handed
	   to `scan` is the pattern's own mandatory tail by construction and cannot
	   drift from it under editing. That construction is THIS file's and not
	   check.mjs's, which is exactly why the agreement between the two is asserted
	   below against a value lifted from check.mjs at test time. */
	const CENTRED = /text-align\s*:\s*center/;
	const CENTER = new RegExp(
		`(?<where>[^{}]{0,200}\\{[^{}]{0,400}?|<[a-z][^<>]{0,300}?)${CENTRED.source}`,
		'i'
	);
	/* `dialog|modal`. THIS COMMENT USED TO CLAIM A LIVE DIVERGENCE — check.mjs's
	   `dialog|modal|toast` against this file's `dialog|modal`, carrying design's
	   2026-08-17 ruling that §13 governs here and that check.mjs "is changing its
	   side to match". IT DID: check.mjs narrowed its own exemption to
	   `dialog|modal` and dates the narrowing in a note at its own rule. The two
	   sides have agreed ever since, and the comment went on describing a
	   divergence that no longer existed — which is why the value is now checked
	   against check.mjs's text rather than against a note about it.

	   The reasoning is kept, because it generalises and because it is why a
	   re-widening has to fail rather than pass: an unused exemption costs nothing
	   to remove, and silently grants everything the day someone builds the
	   component it names. `toast` matched nothing in either tree — a carve-out
	   waiting for its first customer. ⚠️ If the two are ever meant to differ
	   again, that belongs in the assertion below as an explicit inequality with
	   the ruling that authorised it — not here. A note beside a pin is what let
	   the last one rot. */
	const CENTER_EXEMPT: Exemption = { group: 'where', match: /dialog|modal/i };

	/* --- check.mjs's own values, LIFTED FROM ITS TEXT AT TEST TIME -----------
	 *
	 * A guard that encodes a value instead of deriving it decays silently the
	 * moment its source can move independently, and both halves of this rule had:
	 * the pattern was a transcription nothing re-checked, and the exemption
	 * carried a comment describing a divergence check.mjs had already closed.
	 * Reading check.mjs is what makes a change over there fail over here.
	 *
	 * WHAT THIS IS NOT, AND THE DISTINCTION IS THE WHOLE POINT: the lifted values
	 * are never USED as the rule. `CENTER` and `CENTER_EXEMPT` above are still
	 * built here, out of this file's own `CENTRED` fragment, and the lifted pair
	 * is only ever COMPARED against them. A test that ran check.mjs's regex would
	 * assert nothing about agreement — it would agree by construction, and a
	 * mirror is not a guard. Two independently written values, compared, is the
	 * only arrangement that can fail, and it is also the one that leaves room for
	 * a deliberate divergence: one would be written below as an explicit
	 * inequality, argued at the assertion, and would fail on the day check.mjs
	 * closed it. That last property is exactly what the old comment lacked.
	 * ---------------------------------------------------------------------- */
	type Lifted = {
		/** How many matches the extractor found. Asserted BEFORE anything is compared. */
		readonly found: number;
		readonly source: string;
		readonly flags: string;
	};
	/* A JS regex literal, conservatively: any escape, any character class, and no
	   bare `/` outside one. True of both patterns lifted here — and the
	   found-first assertions are what catch the day it stops being. */
	const RE_LITERAL = String.raw`\/((?:[^/\\\n[]|\\.|\[(?:[^\]\\]|\\.)*\])+)\/([a-z]*)`;
	const lift = (text: string, over: RegExp): Lifted => {
		const all = [...text.matchAll(over)];
		return { found: all.length, source: all[0]?.[1] ?? '', flags: all[0]?.[2] ?? '' };
	};
	const CHECK_MJS_SRC = readFileSync(CHECK_MJS, 'utf8');
	/* check.mjs's `const CENTER = /…/i;`. */
	const CHECK_CENTRE = lift(
		CHECK_MJS_SRC,
		new RegExp(String.raw`^const CENTER = ${RE_LITERAL};$`, 'gm')
	);
	/* Its `rule(…, CENTER, …)` call, and the exemption lifted from INSIDE that
	   call rather than from the file — so another rule gaining an exemption is not
	   a failure here. `[^;]` is a negated class, not a dot: it spans newlines. */
	const CHECK_CENTRE_CALLS = [...CHECK_MJS_SRC.matchAll(/\brule\([^;]*?\bCENTER\b[^;]*?\);/g)];
	const CHECK_CENTRE_CALL = CHECK_CENTRE_CALLS[0]?.[0] ?? '';
	const CHECK_CENTRE_EXEMPT = lift(
		CHECK_CENTRE_CALL,
		new RegExp(String.raw`\bmatch:\s*${RE_LITERAL}`, 'g')
	);
	const CHECK_CENTRE_GROUP = /\bgroup:\s*'([a-z]+)'/.exec(CHECK_CENTRE_CALL)?.[1] ?? '';
	/* Reported as `file  selector` rather than `file:line`, because the selector
	   is what a reader has to go and look at. There is no allowlist here and no
	   exception to key one on: `.th__arrow` was the tree's only hit and its
	   declaration was deleted rather than excused. */
	const centreKey = (h: Hit): string => {
		const where = h.groups.where ?? '';
		const brace = where.lastIndexOf('{');
		return `${h.file}  ${collapse(brace === -1 ? where : where.slice(0, brace))}`;
	};

	it('§13 type: no text-align:center outside a dialog', () => {
		const { hits } = applyRule(CENTER, CENTER_EXEMPT, { anchor: CENTRED });
		expect(
			hits.map(centreKey),
			'§13 type: text-align:center outside a dialog.\n' + hits.map(fmt).join('\n')
		).toEqual([]);
	});

	it('§13 type: fires on a planted text-align:center, and the anchor keeps it', () => {
		/* THE DRILL, MADE PERMANENT, and it guards two properties that a green run
		   cannot tell apart on its own. A rule with no hits and a rule that never
		   ran report the same nothing, so the anchor prefilter above — which decides
		   which files the rule runs on at all — is invisible unless something is
		   planted for it to keep. Fired through `applyRule`, the real path, not a
		   re-implementation of it. */
		const planted: readonly Source[] = [
			{ file: 'web/src/planted.css', kind: 'css', src: '.row__cell {\n\ttext-align: center;\n}\n' },
			{
				file: 'web/src/planted-dialog.css',
				kind: 'css',
				src: '.dialog__foot {\n\ttext-align: center;\n}\n'
			},
			{
				file: 'web/src/planted-clean.css',
				kind: 'css',
				src: '.row__cell {\n\ttext-align: start;\n}\n'
			}
		];
		const { hits, exempted } = applyRule(CENTER, CENTER_EXEMPT, {
			anchor: CENTRED,
			over: planted
		});
		expect(hits.map(centreKey), 'the planted declaration did not fire the rule').toEqual([
			'web/src/planted.css  .row__cell'
		]);
		expect(exempted, 'the dialog exemption did not fire on `.dialog__foot`').toBe(1);

		/* The anchor's own mis-wiring guard, fired rather than assumed — an anchor
		   that is not part of the rule must throw, not quietly filter the corpus
		   down to nothing and report a clean tree. */
		expect(() => scan(CENTER, { anchor: /nothing-of-the-sort/ })).toThrow(/mis-wired anchor/);
	});

	it('§13 type: the centring rule still agrees with check.mjs, both halves', () => {
		/* FOUND FIRST, AND THE ORDER IS LOAD-BEARING. An extractor that matched
		   nothing hands back two empty strings, and '' equals '' — the failure mode
		   of this whole arrangement is a green run over a subject that was never
		   located, which is the same defect one level up from the one it replaces.
		   Nothing is compared until the lift is proved to have found something. */
		expect(
			CHECK_CENTRE.found,
			'`const CENTER = /…/;` was not found exactly once in docs/design/check.mjs — ' +
				'the declaration has moved or been renamed and this guard is reading nothing'
		).toBe(1);
		expect(
			CHECK_CENTRE_CALLS.length,
			'the `rule(…, CENTER, …)` call was not found exactly once in docs/design/check.mjs'
		).toBe(1);
		expect(
			CHECK_CENTRE_EXEMPT.found,
			'no `match:` regex inside the CENTER rule call in check.mjs — the exemption has moved'
		).toBe(1);
		/* And that what came back is a whole pattern rather than a truncated one: a
		   lift that stopped at the first `/` or `]` lands well under these floors.
		   Floors, not measurements — they are here to be crossed, not matched. */
		expect(
			CHECK_CENTRE.source.length,
			'lifted CENTER pattern is implausibly short'
		).toBeGreaterThan(60);
		expect(CHECK_CENTRE.source, 'lifted CENTER pattern has no `where` group').toContain(
			'(?<where>'
		);
		expect(
			CHECK_CENTRE_EXEMPT.source.length,
			'lifted centring exemption is implausibly short'
		).toBeGreaterThan(4);
		expect(
			CHECK_CENTRE_GROUP,
			'no `group:` name inside the CENTER rule call in check.mjs — the exemption has moved'
		).toBe('where');

		/* THE AGREEMENT, both halves. Two independently written values: check.mjs's,
		   lifted from its text a moment ago, and this file's, assembled above from
		   `CENTRED`. Either side moving fails this — which is the property the
		   transcribed literal that used to sit here never had. */
		expect(CENTER.source, "CENTER has drifted from check.mjs's pattern").toBe(CHECK_CENTRE.source);
		expect(CENTER.flags, "CENTER's flags have drifted from check.mjs's").toBe(CHECK_CENTRE.flags);
		expect(
			CENTER_EXEMPT.match.source,
			'the centring exemption has drifted from the one in check.mjs. ⚠️ If the divergence is ' +
				'DELIBERATE, do not answer it with a comment — write it here as an explicit ' +
				'inequality naming the ruling that authorised it, so that check.mjs closing ' +
				'the gap fails too. A note beside a pin is exactly how the last one rotted.'
		).toBe(CHECK_CENTRE_EXEMPT.source);
		expect(
			CENTER_EXEMPT.match.flags,
			"the centring exemption's flags have drifted from check.mjs's"
		).toBe(CHECK_CENTRE_EXEMPT.flags);
		expect(
			CENTER_EXEMPT.group,
			'the centring exemption names a different capture group than check.mjs does'
		).toBe(CHECK_CENTRE_GROUP);
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

/* =============================================================================
 * RULE 9 — ARCHITECTURE §17.4 rule 7, over Search's own copy
 *
 * A SEPARATE `describe` BECAUSE THE AUTHORITY IS SEPARATE. Everything above is
 * DESIGN-DIRECTION §13 and reads the whole of `web/src`; this is ARCHITECTURE
 * §17.4 and reads one route. Filing it under the §13 block would have made a
 * failing test name cite the wrong document, which is the sort of small
 * inaccuracy that later gets quoted as an argument.
 * ========================================================================== */

describe('ARCHITECTURE §17.4 rule 7 — over web/src/routes/search', () => {
	it('reads §17.4 rule 7 at all, and both of the strings it defines', () => {
		/* ANTI-VACUITY, AND IT IS THE FIRST TEST FOR A REASON. Every assertion below
		   compares the app against text read out of §17. If that read silently
		   returns nothing — the section renumbered, the rule reworded, the
		   italic-quoted convention abandoned — then `''` is a substring of
		   everything, the drift check passes on any input, and the drill fires on an
		   empty probe. The failure has to happen HERE, where the message can say
		   which of the three reads came back empty. */
		expect(
			s174Body.length,
			'ARCHITECTURE §17.4 could not be located inside §17, so rule 9 is comparing the ' +
				'app against nothing at all'
		).toBeGreaterThan(2000);
		expect(
			rule7,
			`ARCHITECTURE §17.4 rule 7 could not be located. It is matched on its opening ` +
				`words, "${RULE7_HEAD}". If design has reworded the rule, this file follows ` +
				`the wording rather than the other way round: update the marker, then check ` +
				`that the standing wording below is still what ships.`
		).not.toBe('');
		expect(
			STANDING_WORDING,
			'rule 7 was found but carries no *"…"* span, so §17 no longer states the ' +
				'standing wording in the form it reserves for copy that ships. The drift ' +
				'check has nothing to compare against.'
		).not.toBe('');
		expect(
			RETIRED_ENUMERATION,
			'rule 7 was found but no longer records the retired string in backticks. That ' +
				"string is this rule's only positive control: without it the drill below " +
				'fires on nothing and proves nothing.'
		).not.toBe('');
	});

	it('rule 9 vocabulary: every media type the app ships has words', () => {
		/* The seam that keeps the hand-written half honest. `MEDIA_TYPES` is §17.2's
		   navigation enum and the app's own vocabulary; if a seventh lands, this
		   fails and asks for its words rather than letting it go unguarded — which
		   is the silent direction and the one a word list always fails in. */
		expect(
			Object.keys(MEDIA_TYPE_WORDS).sort(),
			'the media-type word list and MEDIA_TYPES have diverged. Add the words for the ' +
				'new type, or remove the ones for the type that went.'
		).toEqual([...MEDIA_TYPES].sort());
		for (const t of MEDIA_TYPES) {
			expect(MEDIA_TYPE_WORDS[t].length, `no words listed for media type "${t}"`).toBeGreaterThan(
				0
			);
		}
	});

	it('reads a Search copy corpus big enough to be the corpus', () => {
		/* 17 strings today, over one file: 11 attributes, 5 markup runs and one
		   module specifier. Floor 12, which is what losing the markup walk costs —
		   the five runs are the copy, the attributes are mostly class names, and a
		   count satisfied by the attribute sweep alone would be the exact hole the
		   per-source floors above exist to refuse. It also fails hard at zero, which
		   is what a renamed route looks like. */
		expect(
			SEARCH_COPY.length,
			`${SEARCH_COPY.length} copy string(s) read under ${SEARCH_ROUTE}, below the floor ` +
				`of 12. Rule 7's corpus is the Search route; a scan that reads none of it ` +
				`reports "no enumeration" forever.`
		).toBeGreaterThanOrEqual(12);
	});

	it('§17.4 rule 7: Search names the corpus as "your services"', () => {
		/* CLAUSE 2, AND IT IS A DRIFT CHECK RATHER THAN A BAN. §17 fixes the wording;
		   this asserts the screen still carries it. Compared with the terminal
		   punctuation trimmed, because §17 quotes two whole sentences and the screen
		   continues the second one ("…and nothing else, and it renders from SQLite"),
		   which is the shipped string agreeing with the standard rather than drifting
		   from it. Everything else — every word, in order — must match. */
		const wanted = norm(STANDING_WORDING).replace(/[.\s]+$/, '');
		const haystack = SEARCH_COPY.map((s) => norm(s.text)).join('\n');
		expect(
			haystack.includes(wanted),
			`§17.4 rule 7: the Search screen no longer carries §17's standing wording for ` +
				`its corpus. §17 fixes it as:\n\n    ${STANDING_WORDING}\n\n` +
				`Nothing under ${SEARCH_ROUTE} contains it. Either the copy has drifted and ` +
				`should be put back, or §17 has been amended and this file is reading the ` +
				`amendment — check which, because only one of those is a bug. Do not soften ` +
				`the comparison to make this green.`
		).toBe(true);
	});

	it('§17.4 rule 7: Search enumerates no media types', () => {
		/* CLAUSE 1, over the real corpus. */
		const found: string[] = [];
		for (const s of SEARCH_COPY) {
			const types = typesNamed(s.text);
			if (types.length >= ENUMERATION_MIN) found.push(reportEnumeration(s, types));
		}
		expect(
			found,
			`§17.4 rule 7: Search's copy enumerates media types.\n\n` +
				`The set of types behind "your services" is whatever the connected services ` +
				`supply, which is a different set on a Prowlarr-only install than on a full ` +
				`stack, so a fixed list is a promise the install may not keep. §17 is explicit ` +
				`that a SHORTER list is the same defect with fewer words: the failure is ` +
				`enumeration, not arity, so do not cut the list down.\n\n` +
				`Name the corpus instead. §17's standing wording is:\n\n    ${STANDING_WORDING}\n\n` +
				`⚠️ And do not add a sentence explaining why an unsupplied type is silent. ` +
				`Nothing in the schema or on the wire separates "no rows matched" from "no ` +
				`source can supply this type", so no string may claim to.\n\n` +
				found.join('\n')
		).toEqual([]);
	});

	it('§17.4 rule 7: fires on the string §17.4 records as its origin', () => {
		/* THE DRILL, MADE PERMANENT, and run through the REAL extractor: the probe is
		   markup, `scanMarkup` turns it into a CopyString exactly as it does for the
		   shipped file, and the same `typesNamed` decides it. A drill against a
		   re-implementation of the rule proves something about the re-implementation.

		   The probe itself is read out of §17.4's backticks rather than retyped, so
		   it is the string that actually shipped until 2026-08-17 and not an
		   approximation of it that happens to be easier to catch. */
		const probe: CopyString[] = [];
		scanMarkup(
			`<div class="pagehead">\n\t<p class="pagehead__meta">\n\t\tThis screen searches ${RETIRED_ENUMERATION}.\n\t</p>\n</div>\n`,
			'web/src/routes/search/planted.svelte',
			probe
		);

		const runs = probe.filter((s) => s.source === 'markup text');
		expect(runs.length, 'the planted markup produced no text run at all').toBe(1);

		const types = typesNamed(runs[0].text);
		expect(
			types,
			'the retired string no longer reads as an enumeration, so the rule has gone ' +
				'inert on the one input it was written for'
		).toEqual(['movies', 'tv', 'music', 'ebooks', 'comics']);
		expect(types.length).toBeGreaterThanOrEqual(ENUMERATION_MIN);

		/* An actionable message names the file, the line, WHICH types tripped it and
		   the text. "no enumeration" would tell the person reading CI none of it.

		   ⚠️ THE LINE IS WHERE THE RUN STARTS, NOT WHERE THE OFFENDING WORD SITS, and
		   that is asserted here rather than left to surprise someone: the probe's
		   dash-free enumeration begins on line 3 and reports line 2, because a text
		   run is located at its opening tag. Over a wrapped paragraph the gap can be
		   several lines. The full text is printed alongside for exactly this reason —
		   the line gets you to the paragraph, the text gets you to the sentence. */
		const message = reportEnumeration(runs[0], types);
		expect(message).toContain('web/src/routes/search/planted.svelte:2');
		expect(message).toContain('markup text');
		expect(message).toContain('names 5 media types');
		expect(message).toContain('movies, tv, music, ebooks, comics');
		expect(message).toContain('film');

		/* THE NEGATIVE HALF, WHICH IS WHAT SEPARATES A RULE FROM AN ASSERTION THAT
		   ALWAYS FAILS. §17's own replacement wording must pass, and so must a single
		   type named once: one mention is not an enumeration and rule 7 does not
		   touch it. If either of these fired, the rule would be unusable and the
		   first person to hit it would add the exception this file has none of. */
		expect(typesNamed(STANDING_WORDING), "§17's own standing wording trips the rule").toEqual([]);
		expect(
			typesNamed('Searching your indexers for a film you do not own is a different thing'),
			'one media type named once reads as an enumeration'
		).toEqual(['movies']);
		expect(
			typesNamed('every film and movie your services already own'),
			'two words for ONE type read as an enumeration of two'
		).toEqual(['movies']);

		/* And the substring trap the alternation ordering exists to close, drilled
		   rather than trusted to the comment: `audiobooks` must not also count as
		   `ebooks`, or a string naming one type fails as though it named two. */
		expect(
			typesNamed('audiobooks are supplied by Audiobookshelf'),
			'`audiobooks` is being read as two types because `books` matched inside it'
		).toEqual(['audiobooks']);
		expect(typesNamed('bookmark this page'), '`book` matched inside `bookmark`').toEqual([]);
	});
});
