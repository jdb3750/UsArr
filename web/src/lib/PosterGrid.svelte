<script lang="ts">
	/**
	 * ONE WRAPPING GRID OF COVER ART, AND THE CARD INSIDE IT.
	 *
	 * WHY THIS IS A COMPONENT NOW. It was inlined in `routes/+page.svelte` with
	 * route-scoped CSS, and that file's own comment named the condition for
	 * lifting it: *"the per-type grids under `routes/library/[type]` are the
	 * second consumer this would need before it is a component"*. They are, and
	 * `routes/library` is a third, so the markup and the CSS moved here whole
	 * rather than being copied twice more.
	 *
	 * THE GRID AND THE CARD ARE ONE COMPONENT AND NOT TWO. Svelte scopes a style
	 * block to the component it sits in, so a card in its own file would need
	 * its container's track rules reached with `:global` — which is exactly the
	 * escape hatch that lets two files' geometry drift apart. There is one box
	 * shape here and one place it is declared.
	 *
	 * ⚠️ THE CARDS ARE INERT, DELIBERATELY, AND THAT IS NOT AN OVERSIGHT. §17.1
	 * requires anything that navigates to be a real `<a href>` that middle-clicks;
	 * the item route `/library/{type}/{id}` these cards would point at does not
	 * exist in `routes/` yet, so a card wrapped in a link would be a link to
	 * nowhere and a card wrapped in a click handler would be the `<div>` that rule
	 * bans. They stay inert until there is somewhere to go, exactly as Home's
	 * always were.
	 *
	 * NO ANIMATION, NO TRANSITION, NO FADE-IN, and nothing here is deferred to an
	 * intersection observer. §17.1 bans animation on any list, grid or navigation
	 * transition, and an image that fades in is the same effect spelt as a load
	 * handler. `loading="lazy"` is the browser's own deferral and costs no script.
	 */
	import { SvelteSet } from 'svelte/reactivity';
	import { NOTHING } from '$lib/list';
	import { posterArtSrc, posterTile, type RecentItem } from '$lib/library';

	let { items }: { items: readonly RecentItem[] } = $props();

	/**
	 * THE WORKS WHOSE ART FAILED TO LOAD IN THIS PAGE VIEW.
	 *
	 * `GET /img/{key}` never fetches upstream: a key whose bytes are not cached
	 * answers `404 not_cached`, which is an ordinary state rather than a fault —
	 * every adapter but BookOrbit writes no poster at all, and a cover that 404'd
	 * upstream leaves the key behind. Without this the browser draws its own
	 * broken-image glyph, which on a screenful of covers reads as a broken screen.
	 *
	 * A SET OF IDS RATHER THAN PER-CARD STATE, because the `{#each}` is keyed by
	 * `work.id` and a card that scrolls out and back must not re-request an image
	 * already known to be missing.
	 */
	const failed = new SvelteSet<number>();
</script>

<div class="postergrid">
	{#each items as item (item.id)}
		{@const tile = posterTile(item)}
		{@const src = posterArtSrc(tile, failed)}
		<div class="postercard">
			<!--
				THE PLACEHOLDER IS A `dominant_color` FILL AND NEVER A GREY BOX, NEVER A
				SHIMMER — §4.4.1 rule 3, and §17.1 and DESIGN-DIRECTION §13 both ban the
				shimmer by name ("it never pulses"). `aspect-ratio` on a `display: block`
				box reserves the space before anything arrives, so a decoded image shifts
				nothing.

				⚠️ WHAT ACTUALLY RENDERS TODAY IS THE FALLBACK, AND THAT IS WORTH SAYING
				RATHER THAN LEAVING TO BE DISCOVERED. `image_asset.dominant_color` is
				declared in migration 00005 and HAS NO WRITER — `docs/ROADMAP.md` says so,
				and `PutPosterAsset`'s INSERT names it nowhere — so `--dc` is set by
				nothing and every empty tile draws `var(--bg-raised)`. The `var(--dc, …)`
				is the seam the column will hang off when something writes it; it is not a
				live feature. A fill sourced from a column with no writer is a live-looking
				thing that never fires, which is the defect class this repo already names
				for guards that cannot trigger.

				THE SAME TILE ANSWERS BOTH ABSENCES. A work with no key and a work whose
				bytes are not cached are different facts about the pipeline and the same
				fact on screen: there is no art. Drawing one of them as a browser glyph
				and the other as a tile would put two renderings of "no cover" on one row.
			-->
			{#if src}
				<!--
					`alt=""` IS DELIBERATE AND IS NOT A GAP. The title is rendered as text
					immediately below, in the same card, so a descriptive alt would make a
					screen reader announce the same work twice. The art is presentation
					here; the title is the content.
				-->
				<img
					class="postercard__art"
					{src}
					alt=""
					title={tile.tooltip}
					loading="lazy"
					decoding="async"
					onerror={() => failed.add(tile.id)}
				/>
			{:else}
				<span class="postercard__art" title={tile.tooltip}></span>
			{/if}
			<!--
				THE TITLE AND YEAR SIT BELOW THE TILE, ON THE CHROME'S OWN GROUND, NEVER
				OVER THE ART (DESIGN-DIRECTION §9.2). Set over the art they would need the
				OKLCh contrast solver §4.4.1 specified, and that solver cannot be made
				safe: it constrains against a SINGLE AVERAGED colour, and real cover art is
				not one colour. Below the tile these are ordinary tokens on a known ground.

				ONE LINE, ELLIPSISED, WITH THE FULL STRING IN `title` — on the art and on
				the title line both. Card height became a function of title length the
				moment the text moved out of the tile, and a wrapped title stands a row of
				cards ragged. Measuring each card to decide whether a tooltip is needed
				would be a layout read per card on the render path, so the attribute is
				unconditional wherever there is a title at all.
			-->
			<div class="postercard__title trunc" title={tile.tooltip}>
				{#if tile.title}{tile.title}{:else}<span class="muted">{NOTHING.empty}</span>{/if}
			</div>
			{#if tile.year !== undefined}
				<div class="postercard__year num">{tile.year}</div>
			{/if}
		</div>
	{/each}
</div>

<style>
	/*
	 * SCOPED TO THIS COMPONENT RATHER THAN ADDED TO app.css, and that is still a
	 * choice about where a shared class earns its place: `web/src/app.css` is a
	 * hand-port of `docs/design/tokens.css` that `tokenparity.test.ts` holds to
	 * it, so a grid that only three screens draw belongs with the markup that
	 * draws it. What changed is that the markup now has one home instead of being
	 * repeated per route.
	 *
	 * `auto-fill` AND NOT `auto-fit`. `auto-fit` collapses the empty tracks and
	 * stretches the last row's cards across the full width, so a page whose final
	 * page holds two items draws two enormous posters. `auto-fill` keeps the track
	 * width and leaves the row short, which is what a grid of fixed artwork should
	 * do.
	 *
	 * A FIXED PX FLOOR ON THE TRACK, NOT A CONTENT-SIZED ONE. `$lib/list`'s
	 * `isContentSized` refuses `auto`, `max-content`, `min-content`,
	 * `fit-content()` and a content-sided `minmax()` on a row grid for a reason
	 * that applies here too: a content-sized track resolves against whatever
	 * happens to be in it, so the column width would move as titles change.
	 */
	.postergrid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(132px, 1fr));
		gap: var(--space-6) var(--space-5);
		align-items: start;
		padding: 0 var(--row-pad-x) var(--space-5);
	}

	.postercard {
		min-width: 0;
	}

	/*
	 * ONE SHAPE FOR EVERY CARD IN THIS GRID (DESIGN-DIRECTION §9.7). 2:3 is the
	 * portrait cover, and a 1:1 album sleeve is FITTED inside it rather than
	 * cropped to it — `contain`, not `cover`, because cropping a square sleeve to
	 * portrait cuts art off both ends.
	 *
	 * ⚠️ THE PER-TYPE GRIDS TAKE THE SAME BOX, AND §9.7 ASKS FOR THE IMAGE'S OWN
	 * SHAPE THERE. The amendment's rule is that a unified grid fits one box while
	 * a single-type grid keeps the image-derived shape — and the shape is not
	 * expressible: `GET /api/v1/library` carries `poster_key` and no dimensions,
	 * so deriving an aspect per item would take a layout read after each decode,
	 * on the render path, which Principle 1 rules out. §9.7's own aspect table is
	 * per media TYPE (2:3 for films and books, 1:1 for albums and artists, 16:9
	 * for episodes) and could be driven off `media_type` — but that is Jellyfin's
	 * rule inverted, branching on the type rather than on the image, and it is a
	 * decision for whoever ships the level control rather than a side effect of
	 * this one. One box until then.
	 *
	 * THE RATIO IS A LITERAL AND NOT A TOKEN, on purpose. There is no aspect-ratio
	 * token in `docs/design/tokens.css`, app.css is a hand-port of that file rather
	 * than a place to invent one, and a single-use value is not what the token
	 * vocabulary is for. The mockups write the same literal.
	 *
	 * `display: block` IS LOAD-BEARING ON THE PLACEHOLDER, not tidiness: the empty
	 * arm is a <span>, `aspect-ratio` does not apply to an inline box, and the tile
	 * collapses to nothing without it.
	 */
	.postercard__art {
		display: block;
		width: 100%;
		height: auto;
		aspect-ratio: 2 / 3;
		object-fit: contain;
		background: var(--dc, var(--bg-raised));
		border: 1px solid var(--border);
	}

	.postercard__title {
		margin-top: var(--space-3);
		font-size: var(--text-base);
		line-height: var(--leading-base);
		color: var(--fg);
	}

	/*
	 * NO `opacity` ON EITHER LINE, and it is a rule rather than a preference
	 * (§4.4.1): a composited opacity changes the effective contrast ratio by a
	 * mechanism no contrast check can see, so the year takes a real colour token.
	 */
	.postercard__year {
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		color: var(--fg-muted);
	}
</style>
