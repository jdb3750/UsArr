/**
 * THE MECHANICS OF THE `?raw` COPY GUARDS, IN ONE PLACE.
 *
 * §17.5 bans specific words about a grab, because a false "failed" invites a
 * user to grab the same 68 GB release twice and a grab is irreversible from
 * UsArr's side. The ban is checked against the strings that actually ship
 * rather than trusted to review — and roughly half of those strings are written
 * inline in a `+page.svelte`, not exported from a module, so the guards read the
 * templates AS TEXT through Vite's `?raw`.
 *
 * ⚠️ THIS MODULE EXISTS BECAUSE THERE ARE NOW TWO GUARDS AND THEY MUST STRIP
 * IDENTICALLY. The grab copy is spread over three files — `RecentGrabs.svelte`
 * holds the rows, `routes/requests/+page.svelte` holds the canonical block, and
 * `routes/+page.svelte` holds Home's summary — and a guard is only as good as
 * the text it is handed. Two hand-copied `replace` chains that drift by one
 * regex give two guards that disagree about what a user can see, which is worse
 * than one guard, because both look green. So the stripping is written once.
 *
 * IT IS TEST SUPPORT AND NOTHING IN THE APPLICATION IMPORTS IT, so it is never
 * reachable from an entry point and never enters the SPA bundle. It lives in
 * `$lib` rather than beside the tests only because `vitest.config.ts` collects
 * `src/**` and there is no other test-support directory yet.
 *
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a
 * component cannot be imported and compiled in a test at all. That is what
 * makes these guards read TEXT, and this module is only the stripping half of
 * it: every export here takes a string and none of them opens a file, so how
 * the caller obtained the text is deliberately not this module's business.
 *
 * WHICH IS WHY BOTH READS FEED IT, AND THE CHOICE IS ABOUT THE QUESTION RATHER
 * THAN ABOUT WHAT THE WORKSPACE SHIPS. Vite's `?raw` answers what ONE NAMED
 * file contains, and `home.test.ts` and `requests.test.ts` hand over templates
 * imported that way; `node:fs` answers what the WHOLE TREE contains, and
 * `havecell.test.ts` walks `web/src` off disk and hands `drawsHaveColumn` the
 * sources it finds. `node:fs` is available here — `web/package.json` carries
 * `@types/node` as a devDependency — and `havecell.test.ts`'s own header sets
 * out why the two reads are not substitutes for each other.
 */

/**
 * The part of a Svelte template a user can actually see.
 *
 * `<script>`, `<style>` and HTML comments all come out — none of them reaches a
 * reader, and a ban list held against a file's own comments would flag the
 * paragraph explaining the ban. Attributes stay in, deliberately: `emptyTitle`,
 * `emptyText`, `loadingNote` and `placeholder` are user-facing strings that live
 * in attributes.
 */
export function userFacingMarkup(source: string): string {
	return source
		.replace(/<script[\s\S]*?<\/script>/gi, ' ')
		.replace(/<style[\s\S]*?<\/style>/gi, ' ')
		.replace(/<!--[\s\S]*?-->/g, ' ');
}

/**
 * EVERY `<section>` on a stripped template whose opening tag carries `marker`,
 * sliced to its own closing tag.
 *
 * ⚠️ ALL OF THEM, NOT THE FIRST, AND THAT IS THE WHOLE REASON THIS IS NOT AN
 * `indexOf` AT THE CALL SITE. Home draws its Recent-grabs region twice — once
 * for the unreadable-list banner and once for the loaded table — as the two arms
 * of one `{#if}`, and both carry `id="home-grabs"`. A slicer that took the first
 * match would have read the error arm and silently dropped the heading, the
 * count and the show-all link, which is a guard with a hole in exactly the half
 * a reader is most likely to see.
 *
 * A MISSING MARKER THROWS RATHER THAN RETURNING NOTHING. The failure mode of a
 * text guard is matching nothing at all: an empty corpus passes every
 * `not.toContain` there is, so a renamed id would turn the ban green instead of
 * red. This is the one place that can tell the difference, so it is the place
 * that fails.
 */
export function sectionsMarkup(markup: string, marker: string): string[] {
	const found: string[] = [];
	let from = 0;
	for (;;) {
		const open = markup.indexOf(marker, from);
		if (open < 0) break;
		const close = markup.indexOf('</section>', open);
		if (close < 0) throw new Error(`the section carrying ${marker} has no closing tag`);
		found.push(markup.slice(open, close));
		from = close;
	}
	if (found.length === 0) throw new Error(`the marker ${marker} is not in the markup any more`);
	return found;
}

/**
 * ONE `{#snippet name(…)}` BLOCK ON A STRIPPED TEMPLATE, TO ITS OWN
 * `{/snippet}`.
 *
 * ⚠️ THIS EXISTS BECAUSE A SECTION SLICE IS NOT THE MARKUP THAT RENDERS THE
 * ROWS, AND A BAN LIST OVER THE WRONG CORPUS IS WORSE THAN NO BAN LIST. Home's
 * Block A guard sliced `id="home-summary"` to `</section>` and asserted that
 * `restricted`, `hidden`, `skeleton`, `shimmer` and `placeholder` were absent
 * from it. Every row of that block is drawn by `{#snippet summaryCellRender}`,
 * which Svelte requires at the TOP LEVEL of the component and which therefore
 * sits outside every `<section>` on the page. All five words were injected into
 * the rendered sub-line of every sourceless row and all 99 tests passed.
 *
 * A SNIPPET IS A TOP-LEVEL BLOCK, SO NESTING IS NOT A CASE. Svelte 5 allows a
 * snippet inside another snippet, but the ones a `+page.svelte` passes as a
 * `cell` prop cannot be nested — they are siblings of the markup — so the first
 * `{/snippet}` after the opener is this one's. A slicer that counted depth would
 * be guessing at a shape the compiler already forbids here.
 *
 * A MISSING SNIPPET THROWS, for `sectionsMarkup`'s reason: an empty corpus
 * passes every `not.toContain` there is, so a renamed snippet would turn a ban
 * green instead of red.
 */
export function snippetMarkup(markup: string, name: string): string {
	const opener = `{#snippet ${name}(`;
	const open = markup.indexOf(opener);
	if (open < 0) throw new Error(`the snippet ${name} is not in the markup any more`);
	const close = markup.indexOf('{/snippet}', open);
	if (close < 0) throw new Error(`the snippet ${name} has no closing tag`);
	return markup.slice(open, close);
}

/**
 * ONE ARM OF AN `{#if}` CHAIN ON A STRIPPED TEMPLATE, SLICED TO ITS OWN SIBLING
 * BOUNDARY — the next `{:else if}`, the next `{:else}`, or the chain's `{/if}`,
 * whichever the nesting reaches first.
 *
 * ⚠️ AN ARM HAS NO CLOSING TOKEN OF ITS OWN, WHICH IS THE WHOLE REASON THIS IS
 * NOT `snippetMarkup`. A snippet and a section each end at a literal a slicer
 * can `indexOf`; an arm ends where its SIBLING opens, and a `{:else}` belonging
 * to an `{#if}` nested inside the arm is not that sibling. So the end is walked
 * — every `{#…}` deepens, every `{/…}` unwinds, and the first `{:…}` or `{/…}`
 * seen at depth zero is the boundary.
 *
 * ⚠️ AND IT IS EMPHATICALLY NOT A FIXED WIDTH, WHICH IS THE DEFECT THAT PUT THIS
 * FUNCTION IN THE FILE. `libraryscreen.test.ts` sliced the Libraries screen's
 * Library cell from `column.id === 'library'` to the end of the file and
 * asserted over the first 1600 characters of that. The arm is 1279 characters
 * long, so the window overran it by 321 characters into the `kind` and `items`
 * arms and the head of `sources`: either assertion could have been satisfied by
 * an arm it was not testing, under a failure message naming the Library cell.
 * Nothing about a character count knows where a branch stops, and the two
 * numbers drift apart every time the markup is edited.
 *
 * A MISSING MARKER THROWS, and so does a missing boundary rather than running on
 * to the end of the markup, for `sectionsMarkup`'s reason: an empty or
 * over-long corpus is exactly what a text guard fails silently on, so the one
 * place that can tell is the place that fails.
 *
 * THE FIRST MATCH WINS, AND A WRONG FIRST MATCH FAILS RED. Pass a marker
 * specific enough to name the arm — the opening tag itself where two arms would
 * otherwise share a prefix. If a duplicate ever shadows the intended arm, the
 * slice is the wrong arm and the assertion over it goes red, which is the
 * direction an ambiguity has to fail in.
 */
export function branchMarkup(markup: string, marker: string): string {
	const open = markup.indexOf(marker);
	if (open < 0) throw new Error(`the branch carrying ${marker} is not in the markup any more`);
	const token = /\{[#/:][a-z]+/g;
	token.lastIndex = open + marker.length;
	let depth = 0;
	for (let hit = token.exec(markup); hit !== null; hit = token.exec(markup)) {
		const sigil = hit[0][1];
		if (sigil === '#') {
			depth += 1;
		} else if (sigil === '/') {
			if (depth === 0) return markup.slice(open, hit.index);
			depth -= 1;
		} else if (depth === 0) {
			return markup.slice(open, hit.index);
		}
	}
	throw new Error(
		`the branch carrying ${marker} has no sibling boundary, so a slice would run to the end of the markup`
	);
}
