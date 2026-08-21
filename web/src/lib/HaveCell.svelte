<script lang="ts">
	/**
	 * THE HAVE CELL — ARCHITECTURE.md §17.2's `have / total · N missing`, through
	 * §6.3's rule and schema.md's polymorphic blob.
	 *
	 * ⚠️ WHY THIS IS A COMPONENT AND NOT MARKUP ON ONE SCREEN. The same cell is
	 * drawn by every screen that lists works — grep the importers rather than
	 * trusting a number here, which is the half that rots. At the extraction
	 * there were two, Home's Block C recently-added table and Search's results
	 * table, drawn by two copies of the same five-arm chain; there are more now,
	 * which is the argument for the component rather than against it. The two
	 * had already diverged, and not in the markup: the markup was byte-identical,
	 * but `.availline`, `.availlabel` and `.availgap` were only ever declared in
	 * Home's own scoped style block, so Search emitted all three class names with
	 * no rule behind any of them. §9.5's whole point — chroma marks what is wrong,
	 * not what is fine — was therefore true on Home and silently absent on Search,
	 * which is the screen where a user most often learns they are missing
	 * something. Styles that live with the markup cannot come apart from it again,
	 * which is why the three rules are at the bottom of this file.
	 *
	 * (Every tag name in this header is spelt in words rather than in angle
	 * brackets. svelte-check's parser scans a whole component for a style tag
	 * instead of skipping the script, so one written here reads as the real one
	 * and the script "was left open" at EOF. The compiler itself does not care,
	 * which is what makes it worth a note: it fails only in the gate.)
	 *
	 * ⚠️ AND THE DECISIONS DID NOT COME WITH IT. `$lib/library.haveCell` owns
	 * every one of them; this renders what it is handed and reconstructs no
	 * comparison of its own. `vitest.config.ts` is `environment: 'node'` with no
	 * Svelte plugin, so nothing in this file can be imported into a test — which
	 * is why `havecell.test.ts` reads it AS TEXT through `?raw`, the same way
	 * `home.test.ts` guards Home's copy. If this file is renamed or moved, that
	 * guard moves with it or the cell silently stops being covered.
	 *
	 * THE PROP BOUNDARY is one fact the caller owns: the row. Both callers pass
	 * the same shape — Search's `SearchItem` IS `RecentItem` (`$lib/search`) —
	 * and `max` is deliberately not a prop, because §9.1's cap of three is a
	 * property of the cell rather than of either screen.
	 *
	 * ⚠️ THE TICK IS THE ONE THING THIS CELL MAY NOT GET WRONG. schema.md is
	 * explicit that `total: null` is not `total: 0` and that §6.3's tick "must
	 * never fire" on the first, so the mark arrives as a discriminated union with
	 * a fourth state for a count with no honest denominator, and this markup
	 * cannot reconstruct a comparison of its own.
	 *
	 * CHROMA MARKS WHAT IS WRONG, NOT WHAT IS FINE (§9.5), which is why the
	 * complete tick is muted and the gap figure is the one thing here carrying
	 * the warn role. Every glyph has a word beside it in the accessibility tree,
	 * because §11 forbids a status glyph with an empty accessible name.
	 *
	 * ⚠️ A ROW NOTHING HAS COUNTED CARRIES NO GLYPH AT ALL, AND THAT IS THE RULE
	 * RATHER THAN A LAYOUT CHOICE. `http-api.md` §1.4.1: an absent `availability`
	 * means no count has ever been computed, so a consumer "must not render an
	 * absent blob as `0`, as "none", or as any glyph, bar or accessible name that
	 * asserts emptiness" — and §1.4.1 calls this out for a results table by name,
	 * "so a result row must not carry an emptiness glyph or an accessible name
	 * like *none held* on the strength of a missing key". The cross belongs to a
	 * PRESENT blob carrying `have: 0`, which is a measured nothing; this one is
	 * words, because there is no glyph for "not yet asked".
	 */
	import Icon from '$lib/Icon.svelte';
	import { haveCell, type RecentItem } from '$lib/library';

	const { item }: { item: RecentItem } = $props();

	const have = $derived(haveCell(item));
</script>

{#each have.lines as line (line.key)}
	<div class="availline">
		{#if line.label}
			<span class="availlabel trunc" title={line.label}>{line.label}</span>
		{/if}
		{#if line.mark.k === 'complete'}
			<span class="muted"><Icon name="check" size="sm" /><span class="sr">complete</span></span>
		{:else if line.mark.k === 'none'}
			<span class="muted"><Icon name="x-circle" size="sm" /><span class="sr">none held</span></span>
		{:else if line.mark.k === 'fraction'}
			<span class="num">{line.mark.have} / {line.mark.total}</span>
		{:else if line.mark.k === 'partial'}
			<span class="num">{line.mark.have}</span>
		{:else}
			<span class="muted">Not counted yet</span>
		{/if}
	</div>
{/each}
{#if have.more > 0}
	<!-- §9.1 caps a cell that renders one line per related object at three plus a
	     count of the rest. A dual-Radarr work has two tiers; a well-catalogued
	     album can have many editions. -->
	<div class="cell-sub">+{have.more} more</div>
{/if}
{#if have.missing}
	<div class="cell-sub availgap">{have.missing}</div>
{/if}
{#if have.gaps}
	<!-- The contiguity list, which is the number that is always honest: it is
	     computed locally from the issue numbers rather than taken from an
	     upstream's declared total. -->
	<div class="cell-sub trunc" title={have.gaps}>Gaps at {have.gaps}</div>
{/if}

<style>
	/*
	 * BLOCK C's Have CELL. A tier- or edition-keyed rollup renders one line per
	 * bucket, so the label and its mark share a line and the lines stack.
	 *
	 * No width is declared on either part. The COLUMN's track is declared (see
	 * each caller's column list) and that is where ADR-0029's alignment
	 * requirement lives; inside the cell, a label that outgrows its share wraps,
	 * which is §9.1's own answer to overflow and is what the column comment means
	 * by letting the cell wrap rather than giving the track an unbounded maximum.
	 */
	.availline {
		display: flex;
		align-items: baseline;
		gap: var(--space-2);
		min-width: 0;
	}

	.availlabel {
		color: var(--fg-muted);
	}

	/*
	 * §9.5, and §17.2 states it for this exact column: chroma marks what is
	 * wrong, not what is fine. So the gap figure is the one thing in this cell
	 * that carries a hue, and the complete tick above is muted rather than
	 * green. The word is present whatever the colour does, which is §9.5's
	 * ordering (icon, text, colour) applied to a cell that has no icon.
	 */
	.availgap {
		color: var(--status-warn);
	}
</style>
