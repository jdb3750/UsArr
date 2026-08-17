<script lang="ts">
	/**
	 * THE RECENT-GRABS TABLE — ARCHITECTURE.md §17.5.
	 *
	 * ⚠️ WHY THIS IS A COMPONENT AND NOT A SNIPPET ON ONE SCREEN. The same rows
	 * are drawn twice: the canonical block on Requests, and Home's narrower
	 * three-column summary. They were drawn by two copies of the same markup, and
	 * the words in that markup are the ones §17.5 bans — a false "failed" invites
	 * the user to grab the same 68 GB release twice, and a grab is irreversible
	 * from UsArr's side. Two copies of banned-vocabulary copy is one copy more
	 * than a guard can see, so there is one.
	 *
	 * ⚠️ AND THE DECISIONS DID NOT COME WITH IT. `vitest.config.ts` is
	 * `environment: 'node'` with no Svelte plugin, so nothing in this file can be
	 * imported into a test: every string, every tone and every action on a row is
	 * `grabOutcome`'s in `$lib/requests`, which a test can read. What is here is
	 * markup, and it renders what it is given. Anything moved INTO this file
	 * leaves the reach of `requests.test.ts` — which is why the copy guard there
	 * reads this file AS TEXT through `?raw` and holds it against the same ban
	 * list as the screen's own markup. If this file is renamed or moved, that
	 * import moves with it or the ban silently stops covering the grab copy.
	 *
	 * THE PROP BOUNDARY, and each prop is a fact the CALLER owns:
	 *
	 *   grabs / total   the rows and the size of the full set. Both callers read
	 *                   the same endpoint and neither may pass `rows.length` as
	 *                   `total` — see List's own note on `aria-rowcount`.
	 *   columns         SIX on Requests, THREE on Home. The projection is the
	 *                   caller's call, not this component's: Home drops Indexer,
	 *                   Protocol and Size for a stated reason each, and this
	 *                   renders whichever cells it is asked for.
	 *   now             the clock the relative column is measured against. Each
	 *                   screen already owns one that moves other things with it,
	 *                   so a second clock in here would tick against them.
	 *   state           the caller's own load state. Requests draws the empty
	 *                   state; Home hides the whole region when empty, so it
	 *                   never reaches one.
	 *   actions         ABSENT ON HOME, AND ABSENT IS THE DEFAULT. §17.3 states a
	 *                   problem canonically once per screen: `Search again` runs
	 *                   a fresh fan-out that only Requests can run, and
	 *                   `Open Services` is Home's Block B job when the fault is
	 *                   UsArr's own configuration. The two arms are gated on ONE
	 *                   prop rather than two, so a caller cannot end up offering
	 *                   half the set — and the callback rides inside it, so the
	 *                   gate and the handler cannot disagree.
	 *
	 * `rowIntrinsic` is NOT a prop. It is `contain-intrinsic-size` for THIS row
	 * shape at the current density, so it is a property of the markup below
	 * rather than of either caller; the number itself is `$lib/requests`'
	 * `RECENT_GRAB_ROW_INTRINSIC`, pinned to app.css's tokens by a test.
	 */
	import { resolve } from '$app/paths';
	import type { RecentGrab } from '$lib/api';
	import List from '$lib/List.svelte';
	import { NOTHING, type ListColumn, type ListState } from '$lib/list';
	import { sizeParts } from '$lib/format';
	import { prefs } from '$lib/prefs.svelte';
	import {
		GRAB_MISSING_TITLE_NOTE,
		RECENT_GRAB_ROW_INTRINSIC,
		formatWhen,
		grabOutcome,
		requestsSearchHref
	} from '$lib/requests';

	interface Props {
		grabs: RecentGrab[];
		columns: ListColumn[];
		now: Date;
		total?: number;
		state?: ListState;
		/**
		 * The follow-up controls a not-sent row may offer, or nothing. Supplying
		 * this declares the caller the canonical block for a grab's follow-up
		 * (§17.3); omitting it renders the same rows with no control on any of
		 * them, which is what a summary is.
		 */
		actions?: { onSearchAgain: (event: MouseEvent, grab: RecentGrab) => void };
	}

	let { grabs, columns, now, total, state = 'default', actions }: Props = $props();

	const requestsPath = resolve('/requests');
	const servicesPath = resolve('/services');

	const rowIntrinsic = $derived(RECENT_GRAB_ROW_INTRINSIC[prefs.density] ?? 44);
</script>

<!--
	`labels`, not `two-line`, below 760 px. §9.1 gives a scanned results list the
	two-line treatment and a record you read one at a time the labelled one, and
	this is the second: the question a row answers is "did that one work?", so
	Outcome may not be one of the fields the two-line fork drops. Ten rows bounds
	what that costs.

	`key` hands the id straight through rather than through String(): it is
	already an opaque string, and nothing on this screen may sort on it, compare
	it numerically or read anything out of its shape.
-->
<List
	label="Recent grabs"
	{columns}
	rows={grabs}
	key={(g: RecentGrab) => g.id}
	{total}
	{rowIntrinsic}
	{state}
	stack="labels"
	loadingNote="Reading your recent grabs."
	emptyTitle="No grabs recorded yet"
	emptyText="A grab you send from a release result is recorded here, newest first, and the record survives a restart — which is what makes it possible to answer “did I already grab that?” an hour later."
>
	{#snippet cell(grab: RecentGrab, column: ListColumn)}
		{#if column.id === 'when'}
			{@const when = formatWhen(grab.grabbedAt, now)}
			{#if when.absolute}
				<!-- Absolute AND relative, per §17.3's rule: one identifies the
				     moment, the other answers "how long ago" without arithmetic. -->
				<span class="num">{when.absolute}</span>
				<div class="cell-sub">{when.relative}</div>
			{:else}
				<span class="muted">{NOTHING.empty}</span>
			{/if}
		{:else if column.id === 'release'}
			{#if grab.releaseTitle}
				<span class="mono trunc" title={grab.releaseTitle}>{grab.releaseTitle}</span>
			{:else}
				<!--
					⚠️ AN EMPTY TITLE IS A FACT HERE, NOT MISSING DATA, so it gets a
					sentence rather than `NOTHING.empty`'s em dash. The candidate had
					already been swept when the audit row was written — `release_
					candidate` is the only place a release name ever lives and it goes
					25 minutes after the search — so UsArr genuinely does not know
					which release this was. An em dash would render that as an
					unremarkable blank; a guessed name would be worse still, since
					recognising the release is the entire job of this column.
				-->
				<span class="muted">{GRAB_MISSING_TITLE_NOTE}</span>
			{/if}
		{:else if column.id === 'indexer'}
			{#if grab.indexerName}
				<span class="trunc" title={grab.indexerName}>{grab.indexerName}</span>
			{:else}
				<span class="muted">{NOTHING.empty}</span>
			{/if}
		{:else if column.id === 'protocol'}
			{#if grab.protocol}
				<span class="proto">{grab.protocol}</span>
			{:else}
				<span class="muted">{NOTHING.empty}</span>
			{/if}
		{:else if column.id === 'size'}
			<!-- A figure and its unit are two slots, not one string — §9.1, the
			     same treatment the release table's Size carries. The reserved box
			     is the half that does the work: right-aligning `4.8 GiB` over
			     `820 MiB` as one string aligns the `B`, so the digits land at two
			     x-positions, and `tabular-nums` cannot help because it is the WORD
			     that moves them. §9.1's exclusion list names `Age` on the two
			     release tables and `Items` on Home; a size column is on neither.

			     3ch, and §9.1 now says 3ch: it reserves the widest unit the column
			     can ever print, and this column prints BINARY units, whose widest
			     member `MiB` is 22px against `MB`'s 19px. §9.1 first derived 2.5ch
			     from the decimal family the design mockups draw; that is corrected
			     there, and `.unit--size` in app.css owns the number.

			     ⚠️ THE `{:else}` ARM IS REACHABLE HERE, unlike on the release
			     table, and it is STRUCTURAL rather than data-dependent:
			     `toNotSentGrabResponse` in internal/httpapi/grabs.go never assigns
			     `SizeBytes` at all, so a not-sent row cannot carry one — the field
			     is `*int64` with `omitempty` and nothing on that path sets it.
			     The release table's field is a plain `int64` with no `omitempty`,
			     always on the wire, so ITS em-dash arm is defensive and
			     unreachable. The two look identical in the markup and are not the
			     same thing. That is why
			     `sizeParts` returning null rather than an empty pair matters: the
			     em dash gets no `.unit`, so no 3ch is held open around nothing.
			     Not every indexer reports a size, and 0 would be a lie rather than
			     an absence. -->
			{@const size = sizeParts(grab.sizeBytes)}
			{#if size}
				{size.value} <span class="unit unit--size">{size.unit}</span>
			{:else}
				<span class="muted">{NOTHING.empty}</span>
			{/if}
		{:else if column.id === 'outcome'}
			{@const outcome = grabOutcome(grab.outcome, {
				errorCode: grab.errorCode,
				hasTitle: grab.releaseTitle !== ''
			})}
			<!--
				THREE STATES, AND THE GROUPING IS THE WHOLE RULE. The two
				handed-over ones both read "sent" and sit beside each other: they
				are the SAME epistemic state about the download, differing only in
				whether an error came back after the handoff. The third reads "not
				sent" and sits beside neither, because nothing was handed over and
				nothing can be running — it is the only state §17.5 permits to be
				worded as "it did not happen".

				Tone follows §9.5's "chroma marks what is wrong, not what is fine":
				neutral on the ordinary sent row, warn where the outcome is unknown
				and worth checking, err where the release never left. The labels
				differ as well as the tone, so removing the colour still leaves all
				three legible.

				There is no Retry on any row, on any state, permanently. And no
				handed-over row offers ANY action: the only control that would fit
				is one that sends the release again, which is exactly what produces
				two copies of a 68 GB release.

				THE SUB-LINES ARE CONDITIONAL, and that is §9.1 rather than a
				nicety: a clause identical on every row of a state is not data,
				and "the client accepted it" under every `sent` chip restated
				the chip at the cost of a second line on every confirmed row.
				$lib/requests decides which state carries what; this renders what
				it is given, and renders nothing when it is given nothing.
			-->
			<span
				class="chip"
				class:chip--pending={outcome.tone === 'warn'}
				class:chip--err={outcome.tone === 'err'}>{outcome.label}</span
			>
			{#if outcome.detail}
				<div class="cell-sub">{outcome.detail}</div>
			{/if}
			{#if outcome.nonAction}
				<!-- §17.5: naming the non-action beats offering a fake one. This
				     line is present exactly where the row's condition is not one
				     the offered control resolves — and on most codes there is no
				     control at all, which is what it says. -->
				<div class="cell-sub">{outcome.nonAction}</div>
			{/if}
			{#if actions}
				{#if outcome.action === 'search-again'}
					<!--
						A LINK, NOT A BUTTON, and the href is real: `?q=` is the
						parameter the Requests screen already seeds a search from, so a
						middle-click opens a tab that runs the search by itself. A plain
						click is intercepted and runs it in place.

						`Search again` rather than `Retry`, and the distinction is not
						cosmetic: this starts a fresh fan-out and posts nothing. Nothing
						on this block re-sends a grab.

						`no-navigation-without-resolve` wants the href to BE a `resolve()`
						call, and a `ResolvedPathname` cannot carry a query string.
						`requestsSearchHref` is §17.4's one construction of that link —
						`resolve('/requests')` with `?q=` appended — so this is a resolved
						path plus the one parameter that makes the link work, and the rule
						is suppressed at the single call site rather than switched off for
						the file.
					-->
					{@const href = requestsSearchHref(requestsPath, grab.releaseTitle)}
					<div class="cell-sub">
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a resolve()'d path plus ?q=, which a ResolvedPathname cannot carry; see $lib/requests requestsSearchHref -->
						<a {href} class="btn btn--sm" onclick={(e) => actions.onSearchAgain(e, grab)}>
							Search again
						</a>
					</div>
				{:else if outcome.action === 'open-services'}
					<!-- Offered only where UsArr's OWN service configuration is what
					     stopped the grab. §17.5 warns that Services is a dead end when
					     the fault is past Prowlarr — it will correctly show green — so
					     it is not offered on the codes that describe Prowlarr's
					     settings or an indexer's answer. -->
					<div class="cell-sub"><a class="btn btn--sm" href={servicesPath}>Open Services</a></div>
				{/if}
			{/if}
		{/if}
	{/snippet}
</List>
