<script lang="ts">
	/**
	 * §17.7's "instance degraded / backend offline" banner, on the catalogue
	 * screens §16.1 funds it for.
	 *
	 * A SIBLING ABOVE `<List>`, never a `<List>` prop. `$lib/degraded`'s header
	 * carries the two reasons — `staleNote` is a `string` and cannot hold this
	 * link, and `ListState` makes `'stale'` and `'empty'` alternatives when the
	 * install that most needs this banner has both at once.
	 *
	 * IT DECIDES NOTHING. Every word comes off `DegradedNotice`, because
	 * `vitest.config.ts` runs in `node` with no Svelte plugin: a rule inside an
	 * `{#if}` here is a rule nothing can execute, so what this template must
	 * contain is guarded by reading it as text — `services-screen.test.ts`'s
	 * idiom, and the reason nothing above is decided in the markup.
	 *
	 * NO ANIMATION, NO FADE, NO SHIMMER, and the reason is the same one §7.2's
	 * Tier 0 rule gives for drawing nothing while a page is in flight: motion in
	 * front of a replica read buys nothing but looks. The styling is `app.css`'s
	 * existing `.banner` family, unchanged and with no custom property added —
	 * `tokenparity.test.ts` fails on any divergence from `design/tokens.css`.
	 */
	import { resolve } from '$app/paths';
	import Icon from '$lib/Icon.svelte';
	import type { DegradedNotice } from '$lib/degraded';

	let { notices }: { notices: DegradedNotice[] } = $props();

	// Resolved HERE rather than taken as a prop, which is `$lib/RecentGrabs`'s
	// pattern for the same link. A path handed in as a string is one
	// `svelte/no-navigation-without-resolve` cannot see through, so the rule
	// would be satisfied by the caller and unenforceable on this file.
	const servicesPath = resolve('/services');
</script>

<!--
	ONE BANNER PER INSTANCE. §17.7 fixes the timestamp as that instance's own, so
	a merged banner would have to quote one instance's number for both.

	`role="status"` rather than `role="alert"`: nothing here interrupts and
	nothing is lost by reading it late — the catalogue below is intact and is
	still being read. The Services screen keeps `alert` for the states that stop
	a user mid-action.

	THE ACTION IS A REAL LINK, not a button that navigates. It is the same exit
	the empty states on these screens already take: Services is the screen that
	owns the fix (§17.3), so this points at it and never grows a second copy of
	its controls.
-->
{#each notices as notice (notice.id)}
	<div class="banner {notice.variant}" role="status">
		<Icon name={notice.icon} />
		<div class="banner__body">
			<div class="banner__title">{notice.name} is {notice.word}</div>
			<p class="banner__text">
				Showing cached data from {notice.stamp}. Everything below is UsArr's own copy of what this
				instance had, so browse, search, sort and filter all still work.
			</p>
		</div>
		<div class="banner__actions">
			<a class="btn" href={servicesPath}>Open Services</a>
		</div>
	</div>
{/each}
