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
 * component cannot be imported and compiled in a test at all. `node:fs` would
 * have been the obvious way to read one and is not available either: this
 * workspace ships no `@types/node`, and adding one to read two files is a
 * dependency for nothing.
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
