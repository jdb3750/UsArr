/*
 * THE HAVE CELL, GUARDED AS TEXT.
 *
 * WHAT THIS FILE IS FOR. The Have column started out drawn on two screens —
 * Home's Block C table and Search's results table — by two copies of one
 * five-arm chain over `haveCell(item)`. Nothing rendered either copy in a test,
 * so the two were free to come apart, and they had: the markup was
 * byte-identical, but `.availline`, `.availlabel` and `.availgap` were declared
 * only in Home's scoped `<style>`, so Search emitted three class names with no
 * rule behind any of them. `$lib/HaveCell.svelte` is now the only copy and this
 * file is what keeps it the only copy — on those two screens, on `/library`
 * which joined them later, and on whatever screen is added next.
 *
 * WHY IT READS SOURCE INSTEAD OF RENDERING. `vitest.config.ts` is
 * `environment: 'node'` with no Svelte plugin, so a component cannot be
 * imported and compiled in a test at all. The repo's answer to that is already
 * in the tree twice: `home.test.ts` reads a template through Vite's `?raw` and
 * runs a ban list over it, and `designrules.test.ts` walks `web/src` off disk.
 * Both idioms are used here, for the two different questions:
 *
 *   `?raw`      is enough to ask what ONE named file contains, and exactly one
 *               file's identity is fixed by this rule: the component itself.
 *   the walk    is the only thing that can ask what the WHOLE TREE contains, and
 *               that is two questions, not one — which files carry the words
 *               (uniqueness), and which route templates draw the column at all
 *               (membership). A `?raw` import can never prove a string is absent
 *               from a file it did not read, and a hand-written list of them can
 *               never check a screen nobody remembered to add to it.
 *
 * ⚠️ THE ROUTE LIST WAS THAT HAND-WRITTEN LIST, AND IT WENT STALE ON THE FIRST
 * NEW SCREEN. `routes/library/+page.svelte` shipped a correct `<HaveCell />` and
 * was never added here, so for as long as it sat outside the list this file
 * would have passed an inline chain on it without a word. The set is derived now,
 * and derived FROM THE THING BEING BANNED, because a selector is where this kind
 * of guard goes deaf: membership is not "routes that import `$lib/library`" nor
 * "routes that render `<HaveCell />`", since both of those are cleared by
 * deleting an import, which is step one of writing the chain by hand. A template
 * joins the set if it does ANY of four things — imports the component, renders
 * it, calls `haveCell()` in its markup, or tests `mark.k` there. The last two ARE
 * the offence, so committing the offence is what enrols the file committing it.
 * There is no way to draw this column, well or badly, without landing in the set.
 *
 * AND THE SCREENS THAT DRAW IT TODAY ARE STILL NAMED, in `NAMED_ROUTES`.
 * Derivation on its own cannot tell "no screen draws a Have column" from "the
 * walk broke", and a guard whose success condition is an absence must never be
 * allowed to confuse the two. The known routes are asserted INTO the derived set
 * before anything is asserted about the set — the same floor `designrules.test.ts`
 * puts under its corpus, written by name rather than as a count.
 *
 * WHAT IS DELIBERATELY NOT HERE. That an absent `availability` blob consults no
 * count, and that `availabilityMark` cannot return `uncounted`, are facts about
 * `$lib/library` and they are tested where they live (`library.test.ts`, the
 * uncounted-is-not-none pair). This file guards the MARKUP contract on top of
 * them: that the words are the fifth arm, that the fifth arm carries no glyph
 * and no number, and that there is exactly one of it.
 */

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { userFacingMarkup } from './copyguard';
import HAVECELL_SOURCE from './HaveCell.svelte?raw';

const HERE = dirname(fileURLToPath(import.meta.url));
const SRC = resolve(HERE, '..');

/** The one file allowed to carry the words, as a `web/src`-relative path. */
const COMPONENT = 'lib/HaveCell.svelte';

/**
 * ⚠️ SPELT IN PIECES SO THIS FILE IS NOT THE SECOND COPY IT IS BANNING. The
 * rule is that the words occur once in `web/src`, and a guard that writes them
 * out to search for them makes that rule unprovable by the obvious `grep` and
 * false by its own text. Any later test needing the string does the same thing.
 */
const UNCOUNTED = ['Not', 'counted', 'yet'].join(' ');

/** The four named marks, in the order the cell tests them. */
const MARKS = ['complete', 'none', 'fraction', 'partial'] as const;

function walk(dir: string, out: string[] = []): string[] {
	for (const entry of readdirSync(dir).sort()) {
		const p = join(dir, entry);
		if (statSync(p).isDirectory()) walk(p, out);
		else out.push(p);
	}
	return out;
}

interface Source {
	readonly file: string;
	readonly src: string;
}

/**
 * Everything under `web/src` that ships or is written by hand. `__fixtures__`
 * is recorded upstream data rather than the product's own voice, which is the
 * same exclusion `designrules.test.ts` makes. Test files are NOT excluded: a
 * second copy of the words in a test is still a second copy, and the split
 * spelling above is how a test avoids being one.
 */
const CORPUS: readonly Source[] = walk(SRC)
	.filter((p) => /\.(ts|svelte|css|html)$/.test(p) && !p.includes('__fixtures__'))
	.map((p) => ({ file: relative(SRC, p).split(/[\\/]/).join('/'), src: readFileSync(p, 'utf8') }));

/**
 * The least credible size of the walk. A scan that matched nothing passes every
 * "occurs once" test by accident, so the corpus is asserted before it is used.
 * Today the walk finds 56 files, none of them generated and none ignored. 40
 * sits far enough below that to survive ordinary editing and nowhere near far
 * enough to survive a broken path or a directory that stops being descended.
 */
const CORPUS_FLOOR = 40;

function occurrences(text: string, needle: string): number {
	let n = 0;
	for (let i = text.indexOf(needle); i >= 0; i = text.indexOf(needle, i + needle.length)) n++;
	return n;
}

/** The one import line that says a template means to use the shared cell. */
const HAVECELL_IMPORT = "from '$lib/HaveCell.svelte'";

/** A `web/src`-relative path to a route template, at any depth. */
const ROUTE_TEMPLATE = /^routes\/(?:.+\/)?\+page\.svelte$/;

/**
 * Does this file draw the Have column — by any means, good or bad?
 *
 * ⚠️ THE FOUR SIGNALS ARE NOT INTERCHANGEABLE AND THE LAST TWO ARE THE POINT.
 * Selecting on the import, or on the rendered element, would mean a screen drops
 * out of the set at the exact moment it stops using the component — which is the
 * moment there is something to catch. `haveCell(` and `mark.k === '…'` in markup
 * are what an inline chain looks like, so a template that hand-rolls this column
 * enrols itself by hand-rolling it, and then fails on both counts below.
 *
 * The import is read off the whole source because it lives in `<script>`; the
 * other three are read off `userFacingMarkup`, so a comment ABOUT the chain — and
 * three route templates carry one, pointing at this component — is not mistaken
 * for the chain. `mark.k === '` is matched with the quote attached: the Libraries
 * screen writes `{#each marks as mark (mark.key)}`, and a bare `mark.k` would
 * drag in a screen that has no availability column at all.
 */
function drawsHaveColumn({ file, src }: Source): boolean {
	if (!ROUTE_TEMPLATE.test(file)) return false;
	if (src.includes(HAVECELL_IMPORT)) return true;
	const markup = userFacingMarkup(src);
	return (
		markup.includes('<HaveCell') ||
		markup.includes('haveCell(') ||
		MARKS.some((m) => markup.includes(`mark.k === '${m}'`))
	);
}

/**
 * The route templates the per-file checks run over, found rather than listed.
 *
 * Everything that keeps this honest is in the file header and in `NAMED_ROUTES`
 * below: a derived set must be floored by name, or "found nothing" reads as
 * "nothing is wrong".
 */
const ROUTES: readonly Source[] = CORPUS.filter(drawsHaveColumn);

/**
 * The screens drawing a Have column today, as a floor under the walk above.
 *
 * ⚠️ THIS IS THE ONLY HAND-MAINTAINED LIST LEFT, AND IT IS DELIBERATELY NOT THE
 * ONE THE CHECKS ITERATE. Its job is the opposite of the old `ROUTES` list's: not
 * to say which files get checked — the walk says that, and it cannot be made to
 * forget a screen — but to fail loudly if the walk stops finding files it has
 * always found. Adding a fifth screen needs no edit here. Deleting one does, and
 * that is the trade: a removal is stated on purpose rather than absorbed in
 * silence.
 */
const NAMED_ROUTES: readonly string[] = [
	'routes/+page.svelte',
	'routes/search/+page.svelte',
	'routes/library/+page.svelte'
];

describe('the corpus this file scans', () => {
	it('found web/src, and found every file the rest of this file names', () => {
		expect(
			CORPUS.length,
			`walked ${SRC} and found only ${CORPUS.length} files, below the floor of ` +
				`${CORPUS_FLOOR}. A scan that matches nothing is not a scan that passed: every ` +
				'uniqueness assertion below would go green on an empty corpus.'
		).toBeGreaterThanOrEqual(CORPUS_FLOOR);

		const files = CORPUS.map((s) => s.file);
		for (const wanted of [COMPONENT, ...NAMED_ROUTES]) {
			expect(files, `${wanted} is not in the walked corpus any more`).toContain(wanted);
		}
	});

	it('found every screen known to draw a Have column, and did not find zero', () => {
		const found = ROUTES.map((s) => s.file);
		expect(
			found,
			`the walk for Have-column screens found ${found.length} of them ` +
				`(${found.join(', ') || 'none'}) and is missing at least one of the ` +
				`${NAMED_ROUTES.length} that have always drawn one. Every per-file check below ` +
				'runs over this list, so a screen that falls out of it stops being checked ' +
				'without failing anything — which is how a derived guard goes deaf. Either the ' +
				'walk broke, or a screen really did drop its Have column and this list is where ' +
				'that gets said out loud.'
		).toEqual(expect.arrayContaining([...NAMED_ROUTES]));
	});
});

describe('the uncounted words exist exactly once in web/src', () => {
	it('only lib/HaveCell.svelte carries them', () => {
		const carriers = CORPUS.filter((s) => s.src.includes(UNCOUNTED)).map((s) => s.file);
		expect(
			carriers,
			`the uncounted-availability words are written in ${carriers.length} files ` +
				`(${carriers.join(', ') || 'none'}). ${COMPONENT} is the one copy, and a second ` +
				'one is how the two screens drifted apart the first time. Render the component ' +
				'instead; if a test needs the words, build them from pieces the way this file does.'
		).toEqual([COMPONENT]);
	});

	it('lib/HaveCell.svelte carries them once, not twice', () => {
		const hits = occurrences(HAVECELL_SOURCE, UNCOUNTED);
		expect(
			hits,
			`${COMPONENT} writes the uncounted-availability words ${hits} times. One arm of ` +
				'one chain says them, so a second occurrence is a second arm nobody reviewed.'
		).toBe(1);
	});
});

describe('every table that draws the Have column renders the shared cell', () => {
	for (const { file, src: source } of ROUTES) {
		it(`${file} imports HaveCell and renders it`, () => {
			expect(
				source,
				`${file} no longer imports the shared Have cell from $lib/HaveCell.svelte. ` +
					'Every table must draw this column from one component.'
			).toContain(HAVECELL_IMPORT);

			expect(
				userFacingMarkup(source),
				`${file} imports the shared Have cell but never renders <HaveCell />. An import ` +
					'with no element is a column drawn by something else.'
			).toContain('<HaveCell');
		});

		it(`${file} has no inline availability chain of its own`, () => {
			const markup = userFacingMarkup(source);
			const inlined = MARKS.filter((m) => markup.includes(`mark.k === '${m}'`));
			expect(
				inlined,
				`${file} tests the availability mark inline (${inlined.join(', ')}). That is the ` +
					'copy-paste this component exists to end: two screens had already come ' +
					'apart on it once. Pass the row to <HaveCell /> and let it decide.'
			).toEqual([]);

			expect(
				markup,
				`${file} calls haveCell() in its own markup. The component owns that call, so ` +
					'no two screens can disagree about what it returns.'
			).not.toContain('haveCell(');
		});
	}
});

describe('the shared cell keeps the availability contract', () => {
	const markup = userFacingMarkup(HAVECELL_SOURCE);

	it('tests the four named marks, in order, and nothing else', () => {
		const positions = MARKS.map((m) => markup.indexOf(`line.mark.k === '${m}'`));
		MARKS.forEach((m, i) => {
			expect(
				positions[i],
				`${COMPONENT} no longer tests the '${m}' mark. All four of ${MARKS.join(', ')} ` +
					'are arms of one chain and the union has no fifth name.'
			).toBeGreaterThanOrEqual(0);
		});
		expect(
			positions,
			`${COMPONENT} tests the marks out of order. 'complete' must be tested first, ` +
				"because §6.3's tick is the one thing this cell may not get wrong."
		).toEqual([...positions].sort((a, b) => a - b));
	});

	it('renders the uncounted row as words, with no glyph and no number', () => {
		const partialAt = markup.indexOf("line.mark.k === 'partial'");
		const elseAt = markup.indexOf('{:else}', partialAt);
		const endAt = markup.indexOf('{/if}', elseAt);
		expect(
			elseAt,
			`${COMPONENT} has no bare {:else} after the 'partial' arm. The row nothing has ` +
				'counted is the fifth arm, and without it an absent blob renders as nothing at all.'
		).toBeGreaterThan(partialAt);
		expect(endAt, `${COMPONENT}'s mark chain is not closed`).toBeGreaterThan(elseAt);

		const arm = markup.slice(elseAt, endAt);
		expect(
			arm,
			`${COMPONENT}'s fifth arm no longer says the uncounted words. http-api.md §1.4.1: ` +
				'an absent availability blob means no count was ever computed, so this arm is ' +
				'the only thing standing between that and a rendered claim about the library.'
		).toContain(UNCOUNTED);
		expect(
			arm,
			`${COMPONENT}'s uncounted arm carries a glyph. http-api.md §1.4.1 forbids it: a ` +
				'row nobody has counted must not render "as any glyph, bar or accessible name ' +
				'that asserts emptiness". The cross belongs to a PRESENT blob holding have: 0.'
		).not.toContain('<Icon');
		for (const field of ['line.mark.have', 'line.mark.total']) {
			expect(
				arm,
				`${COMPONENT}'s uncounted arm reads ${field}. The uncounted mark carries no ` +
					'numbers by construction, and reading one back is how an absent blob starts ' +
					'rendering as a measured zero.'
			).not.toContain(field);
		}
	});

	it('styles the three availability classes it emits', () => {
		for (const cls of ['availline', 'availlabel', 'availgap']) {
			expect(
				HAVECELL_SOURCE,
				`${COMPONENT} emits .${cls} but does not declare it. Svelte scopes a component's ` +
					'styles to that component, so a rule left behind on a route reaches nothing ' +
					'here. That split is exactly how Search ended up with §9.5 silently switched off.'
			).toContain(`.${cls} {`);
		}
	});
});
