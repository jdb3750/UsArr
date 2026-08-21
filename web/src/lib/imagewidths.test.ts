/*
 * THE `?w=` ALLOWLIST, IN THREE PLACES, HELD TO ONE ANSWER.
 *
 * WHAT GOES WRONG WITHOUT THIS. `internal/imagecache` refuses any width that is
 * not on §4.4's list, because an arbitrary `?w=` is a cache-poisoning DoS
 * (GHSA-rrr6-mvwg-9pg9) — so the list is not a convenience, and the server's
 * answer to a width it does not know is a `400` rather than a resize. The list
 * is written out three times in this repository, in two languages, and NOTHING
 * held them together:
 *
 *   1. `internal/imagecache/imagecache.go` — `Width92 … WidthOrig`, which is
 *      what the route actually enforces.
 *   2. `web/src/lib/library.ts` — `IMAGE_WIDTHS`, which is what `posterUrl`
 *      will build a URL out of.
 *   3. `web/src/lib/library.test.ts` — a third hand-copy, asserted as a literal.
 *
 * A width added to (2) with no counterpart in (1) is a broken image on every
 * card that asks for it, and it compiles, type-checks and passes every test in
 * the tree. A width REMOVED from (1) and left in (2) is the same defect with the
 * blame reversed. Neither is a Go bug or a TypeScript bug; it is a bug in the
 * gap between them, which is where nothing was looking.
 *
 * WHY IT READS THE GO SOURCE RATHER THAN CALLING IT. `vitest.config.ts` runs in
 * node and cannot invoke Go, and a fixture recording what Go said would be a
 * FOURTH copy — with the same drift and one more place to update. `readFileSync`
 * over the file is the idiom `api.test.ts` and `tokenparity.test.ts` already use
 * for exactly this shape of question, and it fails on the tree as it stands
 * rather than on a snapshot of it.
 *
 * ⚠️ IT PARSES THE CONST BLOCK, NOT THE MAP AND NOT `Widths()`, and the choice
 * matters. The map and the slice are both spelt in terms of the const names, so
 * a typo in either is a compile error Go already catches; the const block is
 * where the VALUES live, and a value is the thing that reaches the wire. The
 * count is asserted first so a regex that stopped matching cannot pass by
 * finding nothing.
 */

import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { IMAGE_WIDTHS, POSTER_GRID_WIDTH } from './library';

const REPO = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const IMAGECACHE_GO = join(REPO, 'internal/imagecache/imagecache.go');
const LIBRARY_TEST_TS = join(REPO, 'web/src/lib/library.test.ts');

const GO_SOURCE = readFileSync(IMAGECACHE_GO, 'utf8');
const LIBRARY_TEST_SOURCE = readFileSync(LIBRARY_TEST_TS, 'utf8');

/**
 * Every `WidthNNN = "…"` in the file, in declaration order.
 *
 * Declaration order rather than sorted, because the two lists are compared as
 * sequences: `92, 154, 200, 342, 500, 780, orig` is ascending with `orig` last,
 * and a list that agreed as a SET while disagreeing as a sequence would be two
 * different renderings of one allowlist in an error message that prints it.
 */
function goWidths(source: string): string[] {
	const found: string[] = [];
	const pattern = /^\tWidth\w+\s+=\s+"([^"]+)"$/gm;
	for (const match of source.matchAll(pattern)) found.push(match[1]!);
	return found;
}

/** The literal `library.test.ts` asserts `IMAGE_WIDTHS` equals. */
function tsHandCopy(source: string): string[] | undefined {
	const match = /\[\.\.\.IMAGE_WIDTHS\]\)\.toEqual\(\[([^\]]*)\]\)/.exec(source);
	if (match === null) return undefined;
	return [...match[1]!.matchAll(/'([^']*)'/g)].map((m) => m[1]!);
}

describe("the image width allowlist's three copies", () => {
	/*
	 * The floor under everything below. A regex that matched nothing would make
	 * every comparison in this file a comparison of two empty lists.
	 *
	 * ⚠️ IT ASSERTS "NOT EMPTY" AND DELIBERATELY NOT "SEVEN". A count here would
	 * be a FOURTH copy of the allowlist — the exact thing this file exists to
	 * stop — and it would fail on an honestly added width for the wrong reason,
	 * naming the parser when the tree is fine. The comparisons below already fail
	 * on a list that lost or gained a member on one side only, which is the defect;
	 * this only has to prove there is something to compare.
	 */
	it('found all three lists', () => {
		expect(
			goWidths(GO_SOURCE).length,
			`no Width consts parsed out of ${IMAGECACHE_GO} — the const block was reshaped and ` +
				'this test is now comparing nothing to nothing'
		).toBeGreaterThan(0);
		expect(IMAGE_WIDTHS.length, 'IMAGE_WIDTHS is empty').toBeGreaterThan(0);
		expect(
			tsHandCopy(LIBRARY_TEST_SOURCE)?.length ?? 0,
			`the IMAGE_WIDTHS literal is no longer in ${LIBRARY_TEST_TS} in a shape this can read`
		).toBeGreaterThan(0);
	});

	/*
	 * The defect this file exists for, in the direction that hurts most: a width
	 * the browser will ask for and the server will refuse.
	 */
	it('agrees between Go and TypeScript', () => {
		expect(
			[...IMAGE_WIDTHS],
			'IMAGE_WIDTHS and internal/imagecache disagree. A width only TypeScript knows is a ' +
				'400 and a broken image on every card that asks for it; a width only Go knows is ' +
				'a size the UI can never reach'
		).toEqual(goWidths(GO_SOURCE));
	});

	/* The third copy is a literal in another test, so it drifts silently: it
	 * asserts a list rather than deriving one. */
	it("agrees with library.test.ts's hand-copy", () => {
		expect(
			tsHandCopy(LIBRARY_TEST_SOURCE),
			'the literal in library.test.ts no longer matches IMAGE_WIDTHS, so that test is ' +
				'pinning a list this one says is wrong'
		).toEqual([...IMAGE_WIDTHS]);
	});

	/* The width the grid actually sends, checked against the enforcing side
	 * rather than against its own module's list. */
	it('serves the width the poster grid asks for', () => {
		expect(
			goWidths(GO_SOURCE),
			`the grid requests ?w=${POSTER_GRID_WIDTH}, which internal/imagecache does not accept`
		).toContain(POSTER_GRID_WIDTH as string);
	});
});
