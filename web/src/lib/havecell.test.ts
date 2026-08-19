/*
 * THE HAVE CELL, GUARDED AS TEXT.
 *
 * WHAT THIS FILE IS FOR. The Have column is drawn on two screens — Home's
 * Block C table and Search's results table — and it used to be drawn by two
 * copies of one five-arm chain over `haveCell(item)`. Nothing rendered either
 * copy in a test, so the two were free to come apart, and they had: the markup
 * was byte-identical, but `.availline`, `.availlabel` and `.availgap` were
 * declared only in Home's scoped `<style>`, so Search emitted three class names
 * with no rule behind any of them. `$lib/HaveCell.svelte` is now the only copy
 * and this file is what keeps it the only copy.
 *
 * WHY IT READS SOURCE INSTEAD OF RENDERING. `vitest.config.ts` is
 * `environment: 'node'` with no Svelte plugin, so a component cannot be
 * imported and compiled in a test at all. The repo's answer to that is already
 * in the tree twice: `home.test.ts` reads a template through Vite's `?raw` and
 * runs a ban list over it, and `designrules.test.ts` walks `web/src` off disk.
 * Both idioms are used here, for the two different questions:
 *
 *   `?raw`      is enough to ask what ONE named file contains, which is how the
 *               two routes are checked for the component and against the chain.
 *   the walk    is the only thing that can ask what the WHOLE tree contains,
 *               which is the uniqueness question. Three `?raw` imports can
 *               never prove a string is absent from a file they did not read.
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
import HOME_SOURCE from '../routes/+page.svelte?raw';
import SEARCH_SOURCE from '../routes/search/+page.svelte?raw';

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

const ROUTES: readonly (readonly [string, string])[] = [
	['routes/+page.svelte', HOME_SOURCE],
	['routes/search/+page.svelte', SEARCH_SOURCE]
];

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
 * Today the walk finds 53 files, none of them generated and none ignored. 40
 * sits far enough below that to survive ordinary editing and nowhere near far
 * enough to survive a broken path or a directory that stops being descended.
 */
const CORPUS_FLOOR = 40;

function occurrences(text: string, needle: string): number {
	let n = 0;
	for (let i = text.indexOf(needle); i >= 0; i = text.indexOf(needle, i + needle.length)) n++;
	return n;
}

describe('the corpus this file scans', () => {
	it('found web/src, and found the three files the rest of this file names', () => {
		expect(
			CORPUS.length,
			`walked ${SRC} and found only ${CORPUS.length} files, below the floor of ` +
				`${CORPUS_FLOOR}. A scan that matches nothing is not a scan that passed: every ` +
				'uniqueness assertion below would go green on an empty corpus.'
		).toBeGreaterThanOrEqual(CORPUS_FLOOR);

		const files = CORPUS.map((s) => s.file);
		for (const wanted of [COMPONENT, ...ROUTES.map(([f]) => f)]) {
			expect(files, `${wanted} is not in the walked corpus any more`).toContain(wanted);
		}
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

describe('both tables render the shared cell', () => {
	for (const [file, source] of ROUTES) {
		it(`${file} imports HaveCell and renders it`, () => {
			expect(
				source,
				`${file} no longer imports the shared Have cell from $lib/HaveCell.svelte. ` +
					'Both tables must draw this column from one component.'
			).toContain("from '$lib/HaveCell.svelte'");

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
					'copy-paste this component exists to end: the two screens had already come ' +
					'apart on it once. Pass the row to <HaveCell /> and let it decide.'
			).toEqual([]);

			expect(
				markup,
				`${file} calls haveCell() in its own markup. The component owns that call, so ` +
					'the two screens cannot disagree about what it returns.'
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
