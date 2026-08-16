<script lang="ts">
	/**
	 * SEARCH — ARCHITECTURE.md §17.4, AND IT IS NOT BUILT.
	 *
	 * ⚠️ THIS SCREEN USED TO BE THE RELEASE-SEARCH SURFACE and it is not any
	 * more. The Prowlarr fan-out, the results table, the freeze-while-aimed
	 * ordering, the indexer picker and Grab have all MOVED to Requests, where
	 * §17.5 puts the free-text indexer path. They moved rather than being copied:
	 * there is exactly one release-search surface in UsArr, because two results
	 * tables built from one SSE stream is how they end up disagreeing about what
	 * a de-duplicated row is.
	 *
	 * Nothing was lost in the move. The four modules that did the work —
	 * `$lib/frozenorder.svelte`, `$lib/rowstate.svelte`, `$lib/sortspec` and
	 * `$lib/indexerscope.svelte` — were written to be portable and are composed
	 * by Requests unchanged. Only the composition moved.
	 *
	 * ⚠️ SO WHAT IS LEFT HERE IS A GAP, AND IT SAYS SO RATHER THAN FAKING A
	 * SCREEN. §17.4's Search is search over YOUR OWN LIBRARY: one input, results
	 * as a single ranked list, groups ordered by best match, owned results
	 * rendered immediately from the local index. None of that exists yet — the
	 * *Arr library sync is v0.1 and unbuilt, so there is no local index to search
	 * and nothing for a group to hold. An input over it would be the invented
	 * status CLAUDE.md forbids: a control that looks like it works, returns
	 * nothing, and teaches the user their library is empty.
	 *
	 * ⚠️ AND IT IS A SCREEN RATHER THAN A REDIRECT, deliberately. A redirect takes
	 * a working bookmark away and gives no account of where it went. This says
	 * what the screen will be, what it is waiting on, and where the thing the
	 * user was probably looking for is now. The sidebar entry stays for the same
	 * reason — §17.4 is a v0.1 screen, and removing its door would be a status
	 * claim of its own.
	 *
	 * There is no `+layout.svelte` beside this file any more. It carried a
	 * pointer banner over the old results table while both surfaces were
	 * reachable, and it was written to be deleted whole the moment the table
	 * moved. It has been.
	 */
	import { resolve } from '$app/paths';
</script>

<svelte:head><title>Search · UsArr</title></svelte:head>

<!--
	NO <h2> RESTATING THE PAGE NAME. The shell's toolbar already renders it and
	the same string is the h1 inside main, so a heading here would put it on
	screen three times. Standing rule on every screen.
-->

<div class="pagehead">
	<p class="pagehead__meta">
		Search across the library UsArr has replicated from your services. Not built yet — below is what
		it is waiting on, and where to search indexers in the meantime.
	</p>
</div>

<section class="section">
	<!--
		§9.6's empty state, used for the one condition it is genuinely right for.
		This is not "you own nothing" and not "a filter hid everything": it is a
		screen whose data source does not exist yet. One paragraph for what it will
		be, one for what it is waiting on, and the actions go to the things the
		user can actually do today.
	-->
	<div class="empty">
		<h2 class="empty__title">There is no library to search yet</h2>
		<p class="empty__text">
			This screen searches what UsArr has replicated locally — every film, episode, album, book and
			comic your services already own — and renders it from SQLite, so it never waits on an *Arr.
			The replication that fills that index has not shipped, so there is nothing here to match
			against, and an input over it would return nothing for reasons that are UsArr’s rather than
			yours.
		</p>
		<p class="empty__text">
			Searching your <em>indexers</em> for something you do not own is a different thing, and it does
			work today. It lives on Requests, with the sortable results, the indexer flags, the grab and your
			recent grabs beside it.
		</p>
		<div class="empty__actions">
			<a class="btn btn--primary" href={resolve('/requests')}>Search indexers on Requests</a>
			<a class="btn" href={resolve('/services')}>Services</a>
		</div>
	</div>
</section>
